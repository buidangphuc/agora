from __future__ import annotations

from fastapi import APIRouter, Depends

from app.api.v1.ai.dependencies import get_ai_service
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
    MagicListingRequest,
    MagicListingResponse,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
)
from app.modules.business.ai_assistant.service import AIAssistantService

router = APIRouter(tags=["ai"])


@router.post(
    "/assistant",
    response_model=ShoppingAssistantResponse,
    summary="Shopping Assistant (RAG / Catalog Search & Advice)",
    description="Receives natural language buyer questions, searches product catalog via RAG/keyword matching, and returns personalized advice + product cards + suggested followups.",
)
async def shopping_assistant_endpoint(
    payload: ShoppingAssistantRequest,
    ai_service: AIAssistantService = Depends(get_ai_service),
) -> ShoppingAssistantResponse:
    return await ai_service.shopping_assistant(payload)


@router.post(
    "/magic-listing",
    response_model=MagicListingResponse,
    summary="Magic Listing (SEO Title, Description & Pricing Engine)",
    description="Receives title hint / image URL from sellers, and automatically generates SEO-optimized title, comprehensive description, category, price range, and tags.",
)
async def magic_listing_endpoint(
    payload: MagicListingRequest,
    ai_service: AIAssistantService = Depends(get_ai_service),
) -> MagicListingResponse:
    return await ai_service.magic_listing(payload)


@router.post(
    "/chat-copilot",
    response_model=ChatCopilotResponse,
    summary="Chat Copilot (1-Click Smart Replies for Sellers)",
    description="Receives buyer message and generates 3 context-aware, high-converting quick reply options for sellers to send with 1 click.",
)
async def chat_copilot_endpoint(
    payload: ChatCopilotRequest,
    ai_service: AIAssistantService = Depends(get_ai_service),
) -> ChatCopilotResponse:
    return await ai_service.chat_copilot(payload)
