import json
from urllib.request import Request, urlopen
from .models import Quote

class RestSink:
    def __init__(self, base_url: str = "http://127.0.0.1:8080"):
        self.base_url = base_url.rstrip("/")

    def submit_quote(self, quote: Quote) -> None:
        request = Request(self.base_url + "/v1/ingestion/quotes",
                          data=json.dumps(quote.payload()).encode(),
                          headers={"Content-Type": "application/json"}, method="POST")
        with urlopen(request, timeout=10) as response:
            if response.status != 200:
                raise RuntimeError(f"Ingestion failed: {response.status}")
