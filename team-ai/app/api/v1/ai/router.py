from __future__ import annotations

from fastapi import APIRouter, Depends, Query

from app.api.v1.ai.dependencies import get_ai_service, get_tag_classifier_service
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
    MagicListingRequest,
    MagicListingResponse,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
)
from app.modules.business.ai_assistant.service import AIAssistantService
from app.modules.business.tag_classifier.schemas import (
    ClassifyTagsRequest,
    ClassifyTagsResponse,
    ExploreTagsRequest,
    ExploreTagsResponse,
    FacetGroup,
    ListTagsRequest,
    ListTagsResponse,
    PromoteTagRequest,
    PromoteTagResponse,
    TagStatus,
)
from app.modules.business.tag_classifier.service import TagClassifierService

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


# ──────────────────────────────────────────────────────────────────────────────
# Tag Classifier & Filter Taxonomy Enrichment Endpoints
# ──────────────────────────────────────────────────────────────────────────────

@router.post(
    "/tags/classify",
    response_model=ClassifyTagsResponse,
    summary="Classify Product Tags (Online Fast Inference for Seller Posting)",
    description="Fast online inference: extracts and predicts canonical filter tags and emergent candidate tags from title & description to speed up seller listing creation.",
)
async def classify_tags_endpoint(
    payload: ClassifyTagsRequest,
    tag_service: TagClassifierService = Depends(get_tag_classifier_service),
) -> ClassifyTagsResponse:
    return await tag_service.classify_tags(payload)


@router.post(
    "/tags/explore",
    response_model=ExploreTagsResponse,
    summary="Explore Candidate Tags (Offline Discovery & Clustering)",
    description="Offline MLOps pipeline: processes a batch of raw product listings, extracts emergent specs/features, clusters keywords, and registers candidate tags into exploration pool.",
)
async def explore_tags_endpoint(
    payload: ExploreTagsRequest,
    tag_service: TagClassifierService = Depends(get_tag_classifier_service),
) -> ExploreTagsResponse:
    return await tag_service.explore_tags(payload)


@router.post(
    "/tags/promote",
    response_model=PromoteTagResponse,
    summary="Promote Candidate Tags (Gating & Promotion to Canonical Facets)",
    description="Elevates candidate tags that passed confidence/frequency thresholds to official canonical filter facets, enriching search filter aggregations.",
)
async def promote_tags_endpoint(
    payload: PromoteTagRequest,
    tag_service: TagClassifierService = Depends(get_tag_classifier_service),
) -> PromoteTagResponse:
    return await tag_service.promote_tags(payload)


@router.get(
    "/tags",
    response_model=ListTagsResponse,
    summary="List Tags (Canonical Filter Facets & Exploration Candidates)",
    description="Lists marketplace tags filtered by category, facet group (connectivity, material, power, feature), and lifecycle status.",
)
async def list_tags_endpoint(
    category_id: str = Query(default="", description="Filter by category ID"),
    facet_group: FacetGroup | None = Query(default=None, description="Filter by facet group"),
    status: TagStatus | None = Query(default=None, description="Filter by status (promoted, exploring)"),
    tag_service: TagClassifierService = Depends(get_tag_classifier_service),
) -> ListTagsResponse:
    return await tag_service.list_tags(
        ListTagsRequest(
            category_id=category_id,
            facet_group=facet_group,
            status=status,
        )
    )
