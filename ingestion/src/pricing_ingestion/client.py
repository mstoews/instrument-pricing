import json
import os
from urllib.request import Request, urlopen
from .models import Quote

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

    def submit_quote(self, quote: Quote) -> None:
        request = Request(self.base_url + "/v1/ingestion/quotes",
                          data=json.dumps(quote.payload()).encode(),
                          headers=self.headers(), method="POST")
        with urlopen(request, timeout=10) as response:
            if response.status != 200:
                raise RuntimeError(f"Ingestion failed: {response.status}")
