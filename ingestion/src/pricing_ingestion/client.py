import json
import os
from collections.abc import Sequence
from urllib.parse import urlencode
from urllib.request import Request, urlopen
from .models import CurveSnapshot, PriceBar, Quote

MAX_BARS_PER_REQUEST = 5000  # the API's batch limit


class RestSink:
    """Posts normalized records to the Go API with an ingest-scoped API key."""

    def __init__(self, base_url: str = "http://127.0.0.1:8080", api_key: str | None = None):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key if api_key is not None else os.environ.get("PRICING_API_KEY", "")

    def headers(self) -> dict:
        h = {"Content-Type": "application/json"}
        if self.api_key:
            h["Authorization"] = "Bearer " + self.api_key
        return h

    def _call(self, method: str, path: str, body=None):
        data = None if body is None else json.dumps(body).encode()
        request = Request(self.base_url + path, data=data, headers=self.headers(), method=method)
        with urlopen(request, timeout=30) as response:
            return json.load(response)

    def submit_quote(self, quote: Quote) -> None:
        self._call("POST", "/v1/ingestion/quotes", quote.payload())

    def submit_bars(self, bars: Sequence[PriceBar]) -> int:
        """Returns how many bars were new; replays are accepted and skipped."""
        inserted = 0
        for i in range(0, len(bars), MAX_BARS_PER_REQUEST):
            chunk = [b.payload() for b in bars[i:i + MAX_BARS_PER_REQUEST]]
            inserted += self._call("POST", "/v1/ingestion/bars", {"bars": chunk})["inserted"]
        return inserted

    def submit_curve(self, snapshot: CurveSnapshot) -> bool:
        """Returns False when this snapshot was already stored."""
        return self._call("POST", "/v1/ingestion/curves", snapshot.payload())["inserted"]

    def resolve(self, scheme: str, value: str, venue: str = "") -> list[dict]:
        return self._call("GET", "/v1/instruments/resolve?" + urlencode(dict(scheme=scheme, value=value, venue=venue)))

    def create_equity(self, payload: dict) -> str:
        return self._call("POST", "/v1/instruments/equities", payload)["id"]
