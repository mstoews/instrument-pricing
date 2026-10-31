"""Small stdlib HTTP helper: JSON/text GET with retries and client-side pacing."""
import json
import time
from email.utils import parsedate_to_datetime
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

USER_AGENT = "pricing-ingestion/0.2"


class HttpGetter:
    """GET with a minimum interval between calls (vendor rate limits) and
    bounded retries on 429/5xx and connection errors, honouring Retry-After."""

    def __init__(self, min_interval: float = 0.0, retries: int = 4, backoff: float = 2.0,
                 timeout: float = 30.0, sleep=time.sleep, clock=time.monotonic):
        self.min_interval, self.retries, self.backoff, self.timeout = min_interval, retries, backoff, timeout
        self.sleep, self.clock = sleep, clock
        self._last = None

    def _pace(self):
        if self._last is not None and self.min_interval > 0:
            wait = self.min_interval - (self.clock() - self._last)
            if wait > 0:
                self.sleep(wait)
        self._last = self.clock()

    def get(self, url: str, headers: dict | None = None) -> bytes:
        h = {"User-Agent": USER_AGENT, **(headers or {})}
        for attempt in range(self.retries + 1):
            self._pace()
            try:
                with urlopen(Request(url, headers=h), timeout=self.timeout) as r:
                    return r.read()
            except HTTPError as e:
                if e.code != 429 and e.code < 500 or attempt == self.retries:
                    raise
                self.sleep(_retry_after(e) or self.backoff * 2 ** attempt)
            except URLError:
                if attempt == self.retries:
                    raise
                self.sleep(self.backoff * 2 ** attempt)
        raise AssertionError("unreachable")

    def get_json(self, url: str, headers: dict | None = None):
        return json.loads(self.get(url, headers))


def _retry_after(e: HTTPError) -> float | None:
    value = e.headers.get("Retry-After") if e.headers else None
    if not value:
        return None
    try:
        return max(0.0, float(value))
    except ValueError:
        try:
            return max(0.0, parsedate_to_datetime(value).timestamp() - time.time())
        except (TypeError, ValueError):
            return None
