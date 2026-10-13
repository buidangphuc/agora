from dataclasses import dataclass

from app.core.redaction import RedactionPolicy
from app.modules.business.ai_assistant.copilot import ChatCopilotService
from app.modules.business.ai_assistant.listing import ListingCopywriter
from app.modules.business.ai_assistant.retrieval import (
    DemoCatalogRetriever,
    ListingSearch,
    ProductRetriever,
    RagProductRetriever,
)
from app.modules.business.ai_assistant.reviews import ReviewSummarizer
from app.modules.business.ai_assistant.shopping import ShoppingAssistantService


@dataclass(frozen=True, slots=True)
class AIAssistantServices:
    shopping: ShoppingAssistantService
    listing: ListingCopywriter
    copilot: ChatCopilotService
    reviews: ReviewSummarizer


def build_ai_assistant_services(
    rag: ListingSearch | None, *, rag_min_score: float
) -> AIAssistantServices:
    redaction = RedactionPolicy(mode="redacted")
    retriever: ProductRetriever = (
        DemoCatalogRetriever()
        if rag is None
        else RagProductRetriever(rag, min_score=rag_min_score)
    )
    return AIAssistantServices(
        shopping=ShoppingAssistantService(retriever, redaction=redaction),
        listing=ListingCopywriter(redaction=redaction),
        copilot=ChatCopilotService(redaction=redaction),
        reviews=ReviewSummarizer(),
    )
