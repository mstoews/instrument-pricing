import unittest
from datetime import datetime, timezone
from decimal import Decimal
from pricing_ingestion.models import Quote
from pricing_ingestion.adapters.finra import FinraAdapter

class QuoteTests(unittest.TestCase):
    def test_precision(self):
        q = Quote("00000000-0000-0000-0000-000000000001", "sample", datetime.now(timezone.utc), Decimal("1.12345678"), Decimal("2"))
        self.assertEqual(q.payload()["bid"], "1.12345678")
    def test_bad_market_values(self):
        for bid, ask in [("NaN", "2"), ("3", "2"), ("-1", "2")]:
            with self.assertRaises(ValueError):
                Quote("00000000-0000-0000-0000-000000000001", "sample", datetime.now(timezone.utc), Decimal(bid), Decimal(ask)).payload()
    def test_live_adapter_explicitly_unimplemented(self):
        with self.assertRaises(NotImplementedError):
            FinraAdapter().quotes("x")
