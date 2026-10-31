import json
import math
import threading
import unittest
from datetime import date, datetime, timezone
from decimal import Decimal
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from pricing_ingestion.adapters import massive, treasury
from pricing_ingestion.http import HttpGetter
from pricing_ingestion.models import CurveSnapshot, PriceBar

ID = "00000000-0000-0000-0000-000000000001"
FIXTURES = Path(__file__).parent / "fixtures"
DETAILS = {"ticker": "AAPL", "name": "Apple Inc.", "market": "stocks", "locale": "us", "primary_exchange": "XNAS",
           "type": "CS", "active": True, "currency_name": "usd", "cik": "0000320193",
           "composite_figi": "BBG000B9XRY4", "share_class_figi": "BBG001S5N8V8",
           "sic_description": "ELECTRONIC COMPUTERS"}
# 2026-10-29 and 2026-10-30 at midnight New York time (EDT), as Massive stamps daily bars.
T29, T30 = 1793246400000, 1793332800000


class FakeMassive(BaseHTTPRequestHandler):
    """Serves recorded-shape responses; paginates unadjusted bars; throttles once."""
    calls: list = []
    throttle_once = True

    def log_message(self, *args):
        pass

    def _send(self, code, body, headers=None):
        self.send_response(code)
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(json.dumps(body).encode())

    def do_GET(self):
        url = urlparse(self.path)
        q = parse_qs(url.query)
        FakeMassive.calls.append((url.path, q, self.headers.get("Authorization")))
        if FakeMassive.throttle_once:
            FakeMassive.throttle_once = False
            return self._send(429, {"status": "ERROR", "error": "rate limited"}, {"Retry-After": "0"})
        if url.path == "/v3/reference/tickers/AAPL":
            return self._send(200, {"status": "OK", "request_id": "r", "results": DETAILS})
        if url.path == "/v3/reference/tickers/NOPE":
            return self._send(404, {"status": "NOT_FOUND", "message": "Ticker not found."})
        if url.path.startswith("/v2/aggs/ticker/AAPL/range/1/day/"):
            adjusted = q["adjusted"] == ["true"]
            if adjusted:
                return self._send(200, {"status": "OK", "adjusted": True, "results": [
                    {"o": 1, "h": 1, "l": 1, "c": 99.5, "v": 10, "t": T29}, {"o": 1, "h": 1, "l": 1, "c": 101.25, "v": 10, "t": T30}]})
            if "cursor" not in q:
                host = f"http://{self.headers['Host']}"
                return self._send(200, {"status": "DELAYED", "adjusted": False, "next_url": host + url.path + "?cursor=abc&adjusted=false",
                                        "results": [{"o": 100, "h": 101, "l": 99, "c": 100.5, "v": 1234567, "vw": 100.2, "n": 10, "t": T29}]})
            return self._send(200, {"status": "OK", "results": [{"o": 100.5, "h": 102.004, "l": 100.1, "c": 102.0049, "v": 7654321, "t": T30}]})
        self._send(404, {"status": "NOT_FOUND"})


class MassiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 0), FakeMassive)
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def client(self):
        return massive.MassiveClient("secret-key", self.base, http=HttpGetter(backoff=0))

    def test_bar_date_handles_both_timestamp_conventions(self):
        self.assertEqual(massive.bar_date(T30), date(2026, 10, 30))
        midnight_utc = int(datetime(2026, 10, 30, tzinfo=timezone.utc).timestamp() * 1000)
        self.assertEqual(massive.bar_date(midnight_utc), date(2026, 10, 30))

    def test_bars_paginate_join_adjusted_close_and_retry_throttling(self):
        FakeMassive.calls, FakeMassive.throttle_once = [], True
        bars = self.client().bars(ID, "AAPL", date(2026, 10, 29), date(2026, 10, 30))
        self.assertEqual([b.date for b in bars], [date(2026, 10, 29), date(2026, 10, 30)])
        self.assertEqual(bars[0].payload(), dict(instrument_id=ID, source_id="massive", date="2026-10-29", open="100",
                                                 high="101", low="99", close="100.5", volume="1234567", adj_close="99.5"))
        # A close a hair above the reported high is clamped so the bar stays valid.
        self.assertEqual((bars[1].high, bars[1].close, bars[1].adj_close), (Decimal("102.0049"), Decimal("102.0049"), Decimal("101.25")))
        paths = [c[0] for c in FakeMassive.calls]
        self.assertEqual(len(paths), 4)  # throttled, page 1, page 2, adjusted
        self.assertTrue(all(c[2] == "Bearer secret-key" for c in FakeMassive.calls))
        self.assertTrue(all("apiKey" not in c[1] for c in FakeMassive.calls))

    def test_equity_payload_from_ticker_details(self):
        FakeMassive.throttle_once = False
        p = massive.equity_payload(self.client().ticker_details("AAPL"))
        self.assertEqual((p["name"], p["currency"], p["exchange"], p["country"]), ("Apple Inc.", "USD", "XNAS", "US"))
        self.assertIn(dict(scheme="figi", value="BBG000B9XRY4", venue=""), p["identifiers"])
        self.assertEqual(p["identifiers"][0], dict(scheme="ticker", value="AAPL", venue="XNAS"))
        with self.assertRaises(massive.MassiveError):
            massive.equity_payload({**DETAILS, "market": "crypto"})

    def test_vendor_errors_surface(self):
        FakeMassive.throttle_once = False
        with self.assertRaises(Exception):
            self.client().ticker_details("NOPE")
        with self.assertRaises(massive.MassiveError):
            massive.MassiveClient("", self.base)


class HttpGetterTests(unittest.TestCase):
    def test_paces_calls(self):
        now, slept = [0.0], []
        g = HttpGetter(min_interval=12, sleep=lambda s: (slept.append(s), now.__setitem__(0, now[0] + s)), clock=lambda: now[0])
        g._pace(); now[0] += 2; g._pace()
        self.assertEqual(slept, [10])


class TreasuryTests(unittest.TestCase):
    def test_parses_headers_by_tenor_and_skips_blanks(self):
        curves = treasury.parse_par_csv((FIXTURES / "treasury_par.csv").read_text(encoding="utf-8"))
        self.assertEqual([c.date for c in curves], [date(2026, 10, 29), date(2026, 10, 30)])
        self.assertEqual(len(curves[1].yields), 14)
        self.assertNotIn(0.125, curves[0].yields)  # blank 1.5 Month cell
        self.assertAlmostEqual(curves[1].yields[0.125], 0.0418)
        self.assertAlmostEqual(curves[1].yields[30.0], 0.0445)
        with self.assertRaises(ValueError):
            treasury.parse_par_csv("Symbol,Price\nAAPL,1\n")

    def test_flat_par_curve_bootstraps_to_flat_zero_curve(self):
        zeros = dict(treasury.bootstrap({1 / 12: 0.04, 0.5: 0.04, 2: 0.04, 30: 0.04}))
        self.assertAlmostEqual(zeros[30], math.log1p(0.04 / 12) * 12, places=12)  # bill: simple interest
        for days in (182, 365, 3650, 10950):
            self.assertAlmostEqual(zeros[days], 2 * math.log(1.02), places=12)  # semiannual par = zero

    def test_bootstrapped_curve_reprices_par_bonds(self):
        curve = treasury.parse_par_csv((FIXTURES / "treasury_par.csv").read_text(encoding="utf-8"))[1]
        zeros = dict(treasury.bootstrap(curve.yields))
        grid = sorted(d for d in zeros if d >= 182)
        self.assertEqual(len(grid), 60)
        df = [math.exp(-zeros[d] * (n + 1) / 2) for n, d in enumerate(grid)]
        par = sorted(curve.yields.items())
        for n in (3, 19, 59):  # 2, 10 and 30 years
            t = (n + 1) / 2
            c = treasury._interpolate(par, t)
            self.assertAlmostEqual(c / 2 * sum(df[:n + 1]) + df[n], 1.0, places=12)

    def test_snapshot_payload(self):
        curve = treasury.parse_par_csv((FIXTURES / "treasury_par.csv").read_text(encoding="utf-8"))[1]
        p = treasury.snapshot(curve).payload()
        self.assertEqual((p["curve_name"], p["currency"], p["source_id"]), (treasury.CURVE_NAME, "USD", "treasury"))
        self.assertEqual(p["observed_at"], "2026-10-30T19:30:00+00:00")  # 15:30 EDT
        self.assertEqual(p["points"][0], dict(tenor_days=30, zero_rate=f"{math.log1p(0.042 / 12) * 12:.8f}"))  # 1-month bill at 4.20%
        self.assertTrue(all(len(x["zero_rate"].split(".")[1]) == 8 for x in p["points"]))


class ModelTests(unittest.TestCase):
    def test_bar_and_curve_validation(self):
        good = PriceBar(ID, "massive", date(2026, 1, 2), Decimal(1), Decimal(2), Decimal("0.5"), Decimal(1), Decimal(0))
        self.assertNotIn("adj_close", good.payload())
        for bad in [dict(high=Decimal("0.9")), dict(low=Decimal("-1")), dict(adj_close=Decimal(0)), dict(volume=Decimal("NaN"))]:
            with self.assertRaises(ValueError):
                PriceBar(**{**good.__dict__, **bad}).payload()
        now = datetime.now(timezone.utc)
        with self.assertRaises(ValueError):
            CurveSnapshot("c", "USD", "treasury", now, ((30, Decimal("0.04")), (30, Decimal("0.05")))).payload()
        with self.assertRaises(ValueError):
            CurveSnapshot("c", "USD", "treasury", datetime(2026, 1, 1), ((30, Decimal("0.04")),)).payload()
