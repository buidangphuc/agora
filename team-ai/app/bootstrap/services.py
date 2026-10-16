"""Business-service composition: the one place business services are built.

Runs inside lifespan after platform resources are open, so a handler factory
can take whatever it needs (LLM instance, RAG, stores) from ``resources``.
"""

from collections.abc import AsyncIterator, Callable, Sequence
from contextlib import asynccontextmanager

from app.bootstrap.addons.base import BootstrapAddon
from app.bootstrap.resources import (
    ApplicationResources,
    open_application_resources,
    opened,
)
from app.core.config import Settings
from app.core.telemetry import FanOutSink, LogCompletionSink
from app.modules.business.ai_assistant.factory import build_ai_assistant_services
from app.modules.business.chat.factory import build_chat_service
from app.modules.business.completions.guards import CompletionGuard
from app.modules.business.completions.handlers.echo import EchoCompletionHandler
from app.modules.business.completions.ports import CompletionHandler
from app.modules.business.completions.service import (
    CompletionService,
    StreamPolicy,
)
from app.modules.business.listing_screen.factory import build_screen_service
from app.modules.business.recommendation.factory import (
    build_candidate_source,
    build_recommendation_service,
)
from app.modules.business.tag_classifier.service import TagClassifierService

CompletionHandlerFactory = Callable[[ApplicationResources, Settings], CompletionHandler]
CompletionGuardsFactory = Callable[
    [ApplicationResources, Settings], Sequence[CompletionGuard]
]


def default_completion_guards_factory(
    resources: ApplicationResources,
    settings: Settings,
) -> Sequence[CompletionGuard]:
    """Scaffold ships no guards; products add theirs (input/output checks)."""
    return ()


def default_completion_handler_factory(
    resources: ApplicationResources,
    settings: Settings,
) -> CompletionHandler:
    """Scaffold handler; products pass their own factory to ``create_app``."""
    return EchoCompletionHandler()


def compose_business_services(
    resources: ApplicationResources,
    settings: Settings,
    *,
    completion_handler_factory: CompletionHandlerFactory,
    completion_guards_factory: CompletionGuardsFactory,
) -> None:
    resources.completion_service = CompletionService(
        completion_handler_factory(resources, settings),
        guards=completion_guards_factory(resources, settings),
        sink=FanOutSink(
            [
                LogCompletionSink(),
                *([resources.telemetry] if resources.telemetry else []),
            ]
        ),
        policy=StreamPolicy(
            first_token_timeout=settings.STREAM_FIRST_TOKEN_TIMEOUT_SECONDS,
            idle_timeout=settings.STREAM_IDLE_TIMEOUT_SECONDS,
        ),
    )
    resources.ai_assistant = build_ai_assistant_services(
        resources.rag_service, rag_min_score=settings.ASSISTANT_RAG_MIN_SCORE
    )
    resources.chat = build_chat_service(
        settings,
        quota=opened(resources.quota, "quota"),
        redis=opened(resources.redis, "redis"),
    )
    resources.tag_classifier = TagClassifierService(
        store=opened(resources.taxonomy_store, "taxonomy store")
    )
    resources.screen = build_screen_service(settings)
    clients = opened(resources.recommendation_clients, "recommendation clients")
    resources.recommendation = build_recommendation_service(
        settings,
        source=build_candidate_source(settings, clients),
        serving_redis=opened(resources.redis, "redis"),
        clients=clients,
    )


@asynccontextmanager
async def open_runtime(
    settings: Settings,
    *,
    init_resources: bool,
    addons: tuple[BootstrapAddon, ...],
    completion_handler_factory: CompletionHandlerFactory,
    completion_guards_factory: CompletionGuardsFactory,
) -> AsyncIterator[ApplicationResources]:
    """Open resources and compose business services."""
    async with open_application_resources(
        settings,
        init_resources=init_resources,
        addons=addons,
    ) as resources:
        compose_business_services(
            resources,
            settings,
            completion_handler_factory=completion_handler_factory,
            completion_guards_factory=completion_guards_factory,
        )
        await resources.tag_classifier.restore()
        await resources.recommendation.check_ready()
        yield resources
