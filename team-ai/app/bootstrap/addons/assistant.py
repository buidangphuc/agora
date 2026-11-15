"""Lifecycle addon for the shopping assistant: opens its search and listing clients."""

from __future__ import annotations

import importlib.util
from contextlib import AsyncExitStack

import grpc

from app.bootstrap.resources import ApplicationResources
from app.core.config import Settings
from app.modules.business.ai_assistant.adapters.clients import AssistantClients
from app.modules.business.ai_assistant.adapters.listing import GrpcListingReader
from app.modules.business.ai_assistant.adapters.search import GrpcListingSearch


def _scopes(raw: str) -> list[str]:
    return [scope.strip() for scope in raw.split(",") if scope.strip()]


class AssistantAddon:
    name = "assistant"

    def is_enabled(self, settings: Settings) -> bool:
        return True

    async def open(
        self,
        resources: ApplicationResources,
        settings: Settings,
        stack: AsyncExitStack,
    ) -> None:
        if settings.ASSISTANT_BACKEND == "llm_router" and (
            importlib.util.find_spec("langchain_openai") is None
        ):
            raise RuntimeError(
                "ASSISTANT_BACKEND=llm_router needs the ai extra: "
                "`uv sync --extra ai` (or `pip install '.[ai]'`)"
            )
        search = grpc.aio.insecure_channel(settings.RECS_SEARCH_ADDR)
        stack.push_async_callback(search.close)
        listing = grpc.aio.insecure_channel(settings.ASSISTANT_LISTING_ADDR)
        stack.push_async_callback(listing.close)
        resources.assistant_clients = AssistantClients(
            search=GrpcListingSearch(
                search,
                principal_id=settings.RECS_SEARCH_PRINCIPAL_ID,
                principal_scopes=_scopes(settings.RECS_SEARCH_PRINCIPAL_SCOPES),
            ),
            listing=GrpcListingReader(
                listing,
                principal_id=settings.ASSISTANT_LISTING_PRINCIPAL_ID,
                principal_scopes=_scopes(settings.ASSISTANT_LISTING_PRINCIPAL_SCOPES),
            ),
        )
