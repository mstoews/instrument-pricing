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
def get(path):
    with urlopen(base + path, timeout=10) as r:
        return json.load(r)

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
    with urlopen(Request(base + '/v1/pricing/european', data=json.dumps(payload).encode(),
                         headers={'Content-Type': 'application/json'}), timeout=10) as r:
        result = json.load(r)
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
        urlopen(Request(base+path, data=json.dumps(payload).encode(), headers={'Content-Type':'application/json'}), timeout=10)
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
with urlopen(Request(base + '/v1/ingestion/bars', data=json.dumps({'bars': [late]}).encode(), headers={'Content-Type': 'application/json'}), timeout=10) as r:
    assert json.load(r) == {'received': 1, 'inserted': 0}, 'Bar replay overwrote the first payload'
for path, payload, expected in [
    ('/v1/ingestion/bars', {'bars': [dict(late, high='0.5')]}, 400),
    ('/v1/analytics/bond', {'terms': {'maturity': '2031-01-15', 'coupon_rate': '0.05', 'coupon_frequency': 2, 'day_count': '30/360'}, 'settlement': '2031-01-15', 'clean_price': '100'}, 400),
]:
    try:
        urlopen(Request(base+path, data=json.dumps(payload).encode(), headers={'Content-Type':'application/json'}), timeout=10)
        raise AssertionError('Expected rejection')
    except HTTPError as e:
        assert e.code == expected, (e.code, expected)
print('PASS: security master, price bars, bond analytics, performance')
print('PASS: search, resolution, option chain, curves, C++ pricing, Python ingestion, precision, replay, late arrival and error contracts')
