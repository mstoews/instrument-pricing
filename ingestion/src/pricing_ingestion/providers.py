from collections.abc import Iterable
from typing import Protocol
from .models import Quote

class QuoteProvider(Protocol):
    def quotes(self, instrument_id: str) -> Iterable[Quote]: ...

class IngestionSink(Protocol):
    def submit_quote(self, quote: Quote) -> None: ...
