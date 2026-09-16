from datetime import datetime, timezone
from decimal import Decimal
from ..models import Quote

class SampleAdapter:
    def quotes(self, instrument_id: str):
        yield Quote(instrument_id, "sample", datetime.now(timezone.utc),
                    Decimal("201.25"), Decimal("201.35"))
