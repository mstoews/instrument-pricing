from dataclasses import dataclass
from datetime import datetime
from decimal import Decimal
from uuid import UUID

@dataclass(frozen=True)
class Quote:
    instrument_id: str
    source_id: str
    observed_at: datetime
    bid: Decimal
    ask: Decimal

    def payload(self) -> dict:
        UUID(self.instrument_id)
        if (not self.source_id or self.observed_at.utcoffset() is None
            or not self.bid.is_finite() or not self.ask.is_finite()
            or self.bid < 0 or self.ask < self.bid):
            raise ValueError("Invalid normalized quote")
        return dict(instrument_id=self.instrument_id, source_id=self.source_id,
                    observed_at=self.observed_at.isoformat(), bid=format(self.bid, 'f'),
                    ask=format(self.ask, 'f'))
