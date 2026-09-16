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
print('PASS: search, resolution, option chain, curves, C++ pricing, Python ingestion, precision, replay, late arrival and error contracts')
