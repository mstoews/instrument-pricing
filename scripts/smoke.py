"""Integration checks against a migrated, seeded, running API."""
import json
import os
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from urllib.error import HTTPError
from urllib.request import Request, urlopen
from pricing_ingestion.client import RestSink
from pricing_ingestion.models import Quote

base = os.environ.get("API_URL", "http://127.0.0.1:8080")
api_key = os.environ["PRICING_API_KEY"]  # ingest scope; `make smoke` provisions one
auth = {'Authorization': 'Bearer ' + api_key}

def get(path, headers=None):
    with urlopen(Request(base + path, headers=auth if headers is None else headers), timeout=10) as r:
        return json.load(r)

def post(path, payload, headers=None, method='POST'):
    h = {'Content-Type': 'application/json', **(auth if headers is None else headers)}
    with urlopen(Request(base + path, data=json.dumps(payload).encode(), headers=h, method=method), timeout=10) as r:
        return json.load(r)

def status(fn):
    try:
        fn()
    except HTTPError as e:
        return e.code
    return 200

# Authentication: login, refresh rotation, reuse detection, logout, API keys, permissions.
creds = dict(email=os.environ['SMOKE_EMAIL'], password=os.environ['SMOKE_PASSWORD'])
assert status(lambda: get('/v1/instruments?q=AAPL', headers={})) == 401
assert status(lambda: post('/v1/auth/login', dict(creds, password='wrong password'), headers={})) == 401
first = post('/v1/auth/login', creds, headers={})
assert first['token_type'] == 'Bearer' and first['user']['role'] == 'admin'
bearer = lambda pair: {'Authorization': 'Bearer ' + pair['access_token']}
assert get('/v1/auth/me', bearer(first))['credential'] == 'session'
second = post('/v1/auth/refresh', {'refresh_token': first['refresh_token']}, headers={})
assert status(lambda: get('/v1/auth/me', bearer(first))) == 401, 'rotated access token still valid'
assert status(lambda: post('/v1/auth/refresh', {'refresh_token': first['refresh_token']}, headers={})) == 401
assert status(lambda: get('/v1/auth/me', bearer(second))) == 401, 'refresh reuse did not revoke the family'
session = post('/v1/auth/login', creds, headers={})
reader = post('/v1/auth/api-keys', {'name': 'smoke reader', 'scope': 'read'}, headers=bearer(session))
assert reader['key'].startswith('ipk_') and reader['scope'] == 'read'
read_auth = {'Authorization': 'Bearer ' + reader['key']}
assert status(lambda: get('/v1/instruments?q=AAPL', headers=read_auth)) == 200
assert status(lambda: post('/v1/ingestion/quotes', {}, headers=read_auth)) == 403
assert status(lambda: post('/v1/auth/api-keys', {'name': 'x', 'scope': 'read'})) == 403, 'API key reached admin route'
assert any(k['id'] == reader['id'] for k in get('/v1/auth/api-keys', bearer(session)))
req = Request(base + f"/v1/auth/api-keys/{reader['id']}", headers=bearer(session), method='DELETE')
urlopen(req, timeout=10).close()
assert status(lambda: get('/v1/instruments?q=AAPL', headers=read_auth)) == 401, 'revoked key accepted'
post('/v1/auth/logout', {'refresh_token': session['refresh_token']}, headers={})
assert status(lambda: get('/v1/auth/me', bearer(session))) == 401
print('PASS: login, refresh rotation and reuse revocation, logout, API key scopes and revocation, permissions')

i = get("/v1/instruments/resolve?scheme=ticker&value=AAPL&venue=XNAS")[0]
assert i["instrument_type"] == "equity"
assert any(x["id"] == i["id"] for x in get("/v1/instruments?q=AAPL"))
exp = get(f"/v1/options/{i['id']}/expirations")[0]
chain = get(f"/v1/options/{i['id']}/chain?expiration={exp}")
assert len(chain) == 6 and {x['right_type'] for x in chain} == {'call', 'put'}
assert all(Decimal(x['ask']) >= Decimal(x['bid']) for x in chain)
curve = get("/v1/curves?currency=USD")[0]
assert len(get(f"/v1/curves/{curve['id']}/points")) == 2
scenario = dict(exercise_style='european', right_type='call', strike='100',
                time_to_expiry='1', spot='100', volatility='0.20',
                risk_free_rate='0.05', dividend_yield='0')
for right, expected in [('call', '10.450583572'), ('put', '5.573526022')]:
    payload = dict(scenario, right_type=right)
    result = post('/v1/pricing/european', payload)
    assert result['model'] == 'black_scholes' and isinstance(result['price'], str)
    assert abs(Decimal(result['price']) - Decimal(expected)) < Decimal('0.00000001')
sink = RestSink(base)
q = Quote(i['id'], 'sample', datetime.now(timezone.utc), Decimal('202.12345678'), Decimal('202.25'))
sink.submit_quote(q)
first = get(f"/v1/market-data/{i['id']}/quote")
assert Decimal(first['bid']) == q.bid
sink.submit_quote(q)
assert get(f"/v1/market-data/{i['id']}/quote") == first, 'Replay changed receipt time'
sink.submit_quote(Quote(i['id'], 'sample', q.observed_at - timedelta(days=1), Decimal('1'), Decimal('2')))
assert get(f"/v1/market-data/{i['id']}/quote") == first, 'Late arrival overwrote latest observation'
for path, payload, expected in [
    ('/v1/ingestion/quotes', dict(q.payload(), ask='0'), 400),
    ('/v1/ingestion/quotes', dict(q.payload(), source_id='unknown-source'), 400),
    ('/v1/ingestion/trades', {}, 501),
    ('/v1/pricing/european', dict(scenario, exercise_style='american'), 400),
    ('/v1/pricing/european', dict(scenario, risk_free_rate='-1000'), 400),
]:
    try:
        post(path, payload)
        raise AssertionError('Expected rejection')
    except HTTPError as e:
        assert e.code == expected, (e.code, expected)
bond_id = '00000000-0000-0000-0000-000000000301'
bond = get(f"/v1/bonds/{bond_id}")
assert bond['coupon_frequency'] == 2 and bond['calls'][0]['date'] == '2029-05-15'
ba = get(f"/v1/bonds/{bond_id}/analytics?settlement=2026-09-15&source=sample&curve_id={curve['id']}")
assert ba['price_source']['date'] == '2026-09-15' and Decimal(ba['accrued_interest']).quantize(Decimal('0.00001')) == Decimal('1.41667')
assert ba['z_spread'] and Decimal(ba['yield_to_worst']) <= Decimal(ba['yield_to_maturity'])
perf = get(f"/v1/instruments/{i['id']}/performance?source=sample&from=2025-09-15&to=2026-09-15&benchmark_id=00000000-0000-0000-0000-000000000201")
assert perf['observations'] >= 250 and perf['beta'] and perf['sma_200']
bars = get(f"/v1/market-data/{i['id']}/bars?source=sample&from=2026-09-14&to=2026-09-15")
assert [b['date'] for b in bars] == ['2026-09-14', '2026-09-15']
late = dict(instrument_id=i['id'], source_id='sample', date='2026-09-15', open='1', high='1', low='1', close='1')
assert post('/v1/ingestion/bars', {'bars': [late]}) == {'received': 1, 'inserted': 0}, 'Bar replay overwrote the first payload'
for path, payload, expected in [
    ('/v1/ingestion/bars', {'bars': [dict(late, high='0.5')]}, 400),
    ('/v1/analytics/bond', {'terms': {'maturity': '2031-01-15', 'coupon_rate': '0.05', 'coupon_frequency': 2, 'day_count': '30/360'}, 'settlement': '2031-01-15', 'clean_price': '100'}, 400),
]:
    try:
        post(path, payload)
        raise AssertionError('Expected rejection')
    except HTTPError as e:
        assert e.code == expected, (e.code, expected)
snap = dict(curve_name='Smoke zero curve', currency='USD', source_id='treasury', observed_at=datetime.now(timezone.utc).isoformat(),
            points=[dict(tenor_days=30, zero_rate='0.04192667'), dict(tenor_days=3650, zero_rate='-0.0001')])
first_curve = post('/v1/ingestion/curves', snap)
assert first_curve['inserted'] and post('/v1/ingestion/curves', dict(snap, points=snap['points'][:1])) == dict(first_curve, inserted=False)
assert [p['zero_rate'] for p in get(f"/v1/curves/{first_curve['curve_id']}/points")] == ['0.04192667', '-0.0001'], 'curve replay changed points'
print('PASS: security master, price bars, curve ingestion, bond analytics, performance')
print('PASS: search, resolution, option chain, curves, C++ pricing, Python ingestion, precision, replay, late arrival and error contracts')
