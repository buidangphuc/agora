from __future__ import annotations

from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from contextlib import AsyncExitStack

    from app.bootstrap.resources import ApplicationResources
    from app.core.config import Settings


class BootstrapAddon(Protocol):
    name: str

    def is_enabled(self, settings: Settings) -> bool: ...

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        """Open the capability onto ``resources``; push its close onto ``stack``."""
        ...


def default_resource_addons() -> tuple[BootstrapAddon, ...]:
    from app.bootstrap.addons.listing_indexer import ListingIndexerAddon
    from app.bootstrap.addons.quota import QuotaAddon
    from app.bootstrap.addons.rag import RagAddon
    from app.bootstrap.addons.rate_limit import RateLimitAddon
    from app.bootstrap.addons.recommendation import RecommendationAddon
    from app.bootstrap.addons.taxonomy import TaxonomyAddon
    from app.bootstrap.addons.telemetry import TelemetryAddon

    return (
        TelemetryAddon(),
        RateLimitAddon(),
        QuotaAddon(),
        RagAddon(),
        ListingIndexerAddon(),
        TaxonomyAddon(),
        RecommendationAddon(),
    )
