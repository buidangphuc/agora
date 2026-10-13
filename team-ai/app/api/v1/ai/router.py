from typing import Annotated

from fastapi import APIRouter, Depends, Query, Request

from app.bootstrap.state import get_app_resources
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    ChatCopilotResponse,
    MagicListingRequest,
    MagicListingResponse,
    ShoppingAssistantRequest,
    ShoppingAssistantResponse,
)
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifySkuHierarchyResponse,
    ClassifyTagsRequest,
    ClassifyTagsResponse,
    ExploreTagsRequest,
    ExploreTagsResponse,
    ListTagsRequest,
    ListTagsResponse,
    PromoteTagRequest,
    PromoteTagResponse,
)
from app.modules.platform.identity.auth import require_scopes

# The assistant routes take no principal: deliberate parity with team-ai, whose gateway
# authenticates. The tag routes are internal and gated by scope.
router = APIRouter(tags=["ai"])


@router.post("/assistant", response_model=ShoppingAssistantResponse)
async def shopping_assistant(
    payload: ShoppingAssistantRequest, request: Request
) -> ShoppingAssistantResponse:
    return await get_app_resources(request.app).ai_assistant.shopping.answer(payload)


@router.post("/magic-listing", response_model=MagicListingResponse)
async def magic_listing(
    payload: MagicListingRequest, request: Request
) -> MagicListingResponse:
    return get_app_resources(request.app).ai_assistant.listing.generate(payload)


@router.post("/chat-copilot", response_model=ChatCopilotResponse)
async def chat_copilot(
    payload: ChatCopilotRequest, request: Request
) -> ChatCopilotResponse:
    return get_app_resources(request.app).ai_assistant.copilot.suggest(payload)


@router.post(
    "/tags/classify",
    response_model=ClassifyTagsResponse,
    dependencies=[Depends(require_scopes("ai.classify"))],
)
async def classify_tags(
    payload: ClassifyTagsRequest, request: Request
) -> ClassifyTagsResponse:
    return get_app_resources(request.app).tag_classifier.classify_tags(payload)


@router.post(
    "/tags/classify-sku-hierarchy",
    response_model=ClassifySkuHierarchyResponse,
    dependencies=[Depends(require_scopes("ai.classify"))],
)
async def classify_sku_hierarchy(
    payload: ClassifySkuHierarchyRequest, request: Request
) -> ClassifySkuHierarchyResponse:
    return get_app_resources(request.app).tag_classifier.classify_sku_hierarchy(payload)


@router.get(
    "/tags",
    response_model=ListTagsResponse,
    dependencies=[Depends(require_scopes("ai.classify"))],
)
async def list_tags(
    query: Annotated[ListTagsRequest, Query()], request: Request
) -> ListTagsResponse:
    return get_app_resources(request.app).tag_classifier.list_tags(query)


@router.post(
    "/tags/explore",
    response_model=ExploreTagsResponse,
    dependencies=[Depends(require_scopes("admin"))],
)
async def explore_tags(
    payload: ExploreTagsRequest, request: Request
) -> ExploreTagsResponse:
    return await get_app_resources(request.app).tag_classifier.explore_tags(payload)


@router.post(
    "/tags/promote",
    response_model=PromoteTagResponse,
    dependencies=[Depends(require_scopes("admin"))],
)
async def promote_tags(
    payload: PromoteTagRequest, request: Request
) -> PromoteTagResponse:
    return await get_app_resources(request.app).tag_classifier.promote_tags(payload)
