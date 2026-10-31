"""Massive (formerly Polygon.io) daily bars and ticker reference data.

Vendor payloads are parsed only here; the API receives normalized records.
Requests authenticate with `Authorization: Bearer <key>` so the key never
appears in URLs, logs or the `next_url` pagination links.
"""
import os
from collections.abc import Iterable, Iterator
from datetime import date, datetime, timedelta, timezone
from decimal import Decimal
from urllib.parse import quote, urlencode, urljoin

from ..http import HttpGetter
from ..models import PriceBar, Quote

SOURCE_ID = "massive"
# api.polygon.io keeps working after the rebrand; api.massive.com is the new base.
DEFAULT_BASE_URL = "https://api.massive.com"
# The free tier allows 5 requests per minute; paid tiers are effectively unlimited.
DEFAULT_CALLS_PER_MINUTE = 5.0


class MassiveError(RuntimeError):
    pass


def bar_date(t_ms: int) -> date:
    """Session date of a daily aggregate. Massive stamps daily bars at the start
    of the session day; shifting by 12 hours before taking the UTC date gives the
    same day whether that start is midnight New York time or midnight UTC."""
    return (datetime.fromtimestamp(t_ms / 1000, timezone.utc) + timedelta(hours=12)).date()


def normalize_bars(instrument_id: str, raw: Iterable[dict], adjusted: Iterable[dict] = ()) -> list[PriceBar]:
    """Raw (unadjusted) OHLCV plus an optional split-adjusted close keyed by date.
    Massive's adjusted=true is split-adjusted only, not dividend-adjusted."""
    adj = {bar_date(a["t"]): Decimal(str(a["c"])) for a in adjusted}
    bars = []
    for a in raw:
        d = bar_date(a["t"])
        o, h, l, c = (Decimal(str(a[k])) for k in ("o", "h", "l", "c"))
        # Vendor rounding occasionally puts open/close a hair outside high/low.
        h, l = max(h, o, c), min(l, o, c)
        bars.append(PriceBar(instrument_id, SOURCE_ID, d, o, h, l, c,
                             Decimal(str(a.get("v", 0))), adj.get(d)))
    return sorted(bars, key=lambda b: b.date)


def equity_payload(details: dict) -> dict:
    """Map ticker overview (v3 reference) to the POST /v1/instruments/equities body."""
    ticker, venue = details["ticker"], details.get("primary_exchange") or ""
    if details.get("market") != "stocks" or not venue:
        raise MassiveError(f"{ticker}: not an exchange-listed stock (market={details.get('market')!r})")
    identifiers = [dict(scheme="ticker", value=ticker, venue=venue)]
    for field, scheme in (("composite_figi", "figi"), ("share_class_figi", "figi_share_class"), ("cik", "cik")):
        if details.get(field):
            identifiers.append(dict(scheme=scheme, value=str(details[field]), venue=""))
    return dict(name=(details.get("name") or ticker)[:200], currency=(details.get("currency_name") or "usd").upper(),
                exchange=venue, country=(details.get("locale") or "us").upper()[:2],
                sector=(details.get("sic_description") or "")[:100], identifiers=identifiers)


class MassiveClient:
    def __init__(self, api_key: str | None = None, base_url: str | None = None,
                 calls_per_minute: float | None = None, http: HttpGetter | None = None):
        self.api_key = api_key or os.environ.get("MASSIVE_API_KEY", "")
        if not self.api_key:
            raise MassiveError("MASSIVE_API_KEY is not set")
        self.base_url = (base_url or os.environ.get("MASSIVE_BASE_URL") or DEFAULT_BASE_URL).rstrip("/")
        rate = calls_per_minute or float(os.environ.get("MASSIVE_CALLS_PER_MINUTE", DEFAULT_CALLS_PER_MINUTE))
        self.http = http or HttpGetter(min_interval=60.0 / rate)

    def _get(self, url: str) -> dict:
        body = self.http.get_json(url, {"Authorization": f"Bearer {self.api_key}"})
        if body.get("status") not in ("OK", "DELAYED"):
            raise MassiveError(body.get("error") or body.get("message") or f"unexpected status {body.get('status')!r}")
        return body

    def ticker_details(self, ticker: str) -> dict:
        return self._get(f"{self.base_url}/v3/reference/tickers/{quote(ticker)}")["results"]

    def daily_aggregates(self, ticker: str, start: date, end: date, adjusted: bool) -> Iterator[dict]:
        params = urlencode(dict(adjusted=str(adjusted).lower(), sort="asc", limit=50000))
        url = f"{self.base_url}/v2/aggs/ticker/{quote(ticker)}/range/1/day/{start}/{end}?{params}"
        while url:
            body = self._get(url)
            yield from body.get("results") or []
            nxt = body.get("next_url")
            # Pagination links are absolute; keep them on the configured host.
            url = urljoin(self.base_url + "/", nxt) if nxt else None

    def bars(self, instrument_id: str, ticker: str, start: date, end: date) -> list[PriceBar]:
        raw = list(self.daily_aggregates(ticker, start, end, adjusted=False))
        adjusted = list(self.daily_aggregates(ticker, start, end, adjusted=True)) if raw else []
        return normalize_bars(instrument_id, raw, adjusted)


class MassiveAdapter:
    """Quotes are not implemented: the free and starter tiers are end-of-day."""
    def quotes(self, instrument_id: str) -> Iterable[Quote]:
        raise NotImplementedError("massive live quotes are not implemented; use bars")
