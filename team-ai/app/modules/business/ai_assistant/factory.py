import importlib.util
from dataclasses import dataclass

from app.core.config import Settings
from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.copilot_llm import LLMCopilotDrafter
from app.modules.business.ai_assistant.grounding import (
    GroundedCards,
    ListingGrounding,
    ListingPort,
)
from app.modules.business.ai_assistant.listing import ListingCopywriter
from app.modules.business.ai_assistant.reviews import ReviewSummarizer
from app.modules.business.ai_assistant.shopping import (
    AssistantReplier,
    MockAssistantReplier,
    ShoppingAssistantService,
)


@dataclass(frozen=True, slots=True)
class AIAssistantServices:
    shopping: ShoppingAssistantService
    listing: ListingCopywriter
    copilot: ChatCopilotService
    reviews: ReviewSummarizer


class _UngroundedSearch:
    async def ground(self, query: str) -> GroundedCards:
        return GroundedCards(failed=True)


def build_ai_assistant_services(
    *,
    grounding: ListingGrounding | None = None,
    replier: AssistantReplier | None = None,
) -> AIAssistantServices:
    redaction = RedactionPolicy(mode="redacted")
    return AIAssistantServices(
        shopping=ShoppingAssistantService(
            grounding or _UngroundedSearch(),  # type: ignore[arg-type]
            replier or MockAssistantReplier(),
            redaction=redaction,
        ),
        listing=ListingCopywriter(redaction=redaction),
        copilot=ChatCopilotService(redaction=redaction),
        reviews=ReviewSummarizer(),
    )


def build_copilot_service(
    settings: Settings, listing_lookup: ListingPort | None
) -> ChatCopilotService:
    redaction = RedactionPolicy(mode="redacted")
    drafter = None
    if settings.COPILOT_BACKEND == "llm_router":
        if importlib.util.find_spec("langchain_core") is None:
            raise RuntimeError(
                "COPILOT_BACKEND=llm_router needs the ai extra: "
                "`uv sync --extra ai` (or `pip install '.[ai]'`)"
            )
        from app.modules.ai.llm import runtime

        llm = runtime.build_llm_instance(
            settings,
            instance_id="grpc-copilot",
            service_name="team-ai.copilot",
            tags=("grpc", "copilot"),
        )
        drafter = LLMCopilotDrafter(
            llm, timeout_seconds=settings.COPILOT_TIMEOUT_SECONDS
        )
    return ChatCopilotService(
        redaction=redaction,
        listing=listing_lookup,
        drafter=drafter,
        listing_timeout_s=settings.ASSISTANT_LISTING_TIMEOUT_SECONDS,
    )
