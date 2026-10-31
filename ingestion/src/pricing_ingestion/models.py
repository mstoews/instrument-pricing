from dataclasses import dataclass
from datetime import date, datetime
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


def _decimal(value: Decimal) -> str:
    return format(value, 'f')


@dataclass(frozen=True)
class PriceBar:
    """A daily bar. adj_close is optional (None when the source has none)."""
    instrument_id: str
    source_id: str
    date: date
    open: Decimal
    high: Decimal
    low: Decimal
    close: Decimal
    volume: Decimal
    adj_close: Decimal | None = None

    def payload(self) -> dict:
        UUID(self.instrument_id)
        prices = [self.open, self.high, self.low, self.close]
        if (not self.source_id or not all(p.is_finite() and p >= 0 for p in prices)
                or not (self.low <= min(self.open, self.close) and max(self.open, self.close) <= self.high)
                or not self.volume.is_finite() or self.volume < 0
                or (self.adj_close is not None and not (self.adj_close.is_finite() and self.adj_close > 0))):
            raise ValueError(f"Invalid normalized bar for {self.date}")
        out = dict(instrument_id=self.instrument_id, source_id=self.source_id, date=self.date.isoformat(),
                   open=_decimal(self.open), high=_decimal(self.high), low=_decimal(self.low),
                   close=_decimal(self.close), volume=_decimal(self.volume))
        if self.adj_close is not None:
            out['adj_close'] = _decimal(self.adj_close)
        return out


@dataclass(frozen=True)
class CurveSnapshot:
    """Continuously compounded zero rates (decimal fractions) by tenor in days."""
    curve_name: str
    currency: str
    source_id: str
    observed_at: datetime
    points: tuple[tuple[int, Decimal], ...]

    def payload(self) -> dict:
        tenors = [t for t, _ in self.points]
        if (not self.curve_name or len(self.currency) != 3 or not self.source_id
                or self.observed_at.utcoffset() is None or not self.points
                or len(set(tenors)) != len(tenors) or min(tenors) <= 0
                or not all(r.is_finite() for _, r in self.points)):
            raise ValueError("Invalid normalized curve snapshot")
        return dict(curve_name=self.curve_name, currency=self.currency, source_id=self.source_id,
                    observed_at=self.observed_at.isoformat(),
                    points=[dict(tenor_days=t, zero_rate=_decimal(r)) for t, r in self.points])
