from app.modules.business.listing_screen.ports import ScreenResult, ScreenVerdict

MOCK_LABEL = "mock_marker"


class MockScreenService:
    """Deterministic, offline: REVIEW iff the text contains the marker."""

    def __init__(self, marker: str) -> None:
        self._marker = marker.casefold()

    async def screen(
        self,
        *,
        title: str,
        description: str,
        listing_id: str = "",
        category_id: str = "",
    ) -> ScreenResult:
        text = f"{title}\n{description}".casefold()
        if self._marker and self._marker in text:
            return ScreenResult(ScreenVerdict.REVIEW, (MOCK_LABEL,), "mock")
        return ScreenResult(ScreenVerdict.ALLOW, (), "mock")
