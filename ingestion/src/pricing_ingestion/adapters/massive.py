from collections.abc import Iterable
from ..models import Quote

class MassiveAdapter:
    """Placeholder: implement vendor transport and normalization here.

    No live API request is made. Provider-specific capabilities (FINRA trades,
    FRED curve observations) need typed protocols before implementation.
    """
    def quotes(self, instrument_id: str) -> Iterable[Quote]:
        raise NotImplementedError("massive live adapter is not implemented; use sample")
