from collections.abc import Sequence
from dataclasses import dataclass
from enum import Enum
from typing import Protocol


class ScreenVerdict(str, Enum):
    ALLOW = "allow"
    REVIEW = "review"


@dataclass(frozen=True, slots=True)
class ScreenResult:
    verdict: ScreenVerdict
    labels: Sequence[str]
    backend: str


class ScreenService(Protocol):
    async def screen(
        self,
        *,
        title: str,
        description: str,
        listing_id: str = "",
        category_id: str = "",
    ) -> ScreenResult:
        """Screen one listing's text; raises ``ServiceUnavailableError``."""
        ...
