from dataclasses import dataclass

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.grounding import GroundedCards, ListingGrounding
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
