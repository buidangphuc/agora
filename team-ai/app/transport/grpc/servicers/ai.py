"""AIService gRPC servicer (ShoppingAssistant, MagicListing, ChatCopilot, reviews).

Adheres to the platform-core gRPC contract (platform.ai.v1.AIService). Delegates to
the rule-based ``AIAssistantService`` (no LLM / RAG).

Access control: every RPC calls ``ensure_scopes`` as its FIRST statement, with the
scopes from ``app.transport.grpc.scopes`` (MagicListing/ChatCopilot need
``listing.write``, SummarizeReviews needs ``listing.read``; ShoppingAssistant needs
``ai:use`` once ``AI_USE_SCOPE_REQUIRED`` is on, which team-identity does not yet
grant). ChatCopilot additionally rejects a ``seller_id`` that is not the caller's
own (admin exempt). Only ShoppingAssistant is rate-limited (opt-in interceptor).

Errors: handler bodies run under ``map_errors`` — invalid input becomes
INVALID_ARGUMENT naming only the fields, a known unavailability becomes
UNAVAILABLE, anything else INTERNAL with the fixed text ``internal error``; the
original exception is logged with the request id and never sent to the caller.
The scope checks sit outside that wrapper so an abort is never re-mapped.
"""

from __future__ import annotations

from collections.abc import Callable

import grpc

from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    ReviewItem,
    ShoppingAssistantRequest,
    SummarizeReviewsRequest,
)
from app.modules.business.ai_assistant.service import AIAssistantService
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest as TagsRequest,
    SkuVariantInput,
    TagItem,
)
from app.modules.business.tag_classifier.service import (
    TagClassifierService,
    shared_tag_classifier,
)
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.errors import map_errors
from app.transport.grpc.scopes import ADMIN_SCOPE, AI_SERVICE_SCOPES, ai_use_scopes

AIProvider = Callable[[], AIAssistantService]
TagClassifierProvider = Callable[[], TagClassifierService]


class AIServicer(ai_pb2_grpc.AIServiceServicer):
    def __init__(
        self,
        ai_provider: AIProvider | None = None,
        *,
        require_ai_use: bool = False,
        tag_classifier_provider: TagClassifierProvider | None = None,
    ) -> None:
        self._ai_provider = ai_provider or (lambda: AIAssistantService())
        self._tag_provider = tag_classifier_provider or shared_tag_classifier
        self._shopper_scopes = ai_use_scopes(require_ai_use)

    async def ShoppingAssistant(
        self,
        request: ai_pb2.ShoppingAssistantRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ShoppingAssistantResponse:
        await ensure_scopes(context, *self._shopper_scopes)
        return await self._shopping_assistant(request, context)

    @map_errors("ShoppingAssistant")
    async def _shopping_assistant(
        self,
        request: ai_pb2.ShoppingAssistantRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ShoppingAssistantResponse:
        ai_service = self._ai_provider()
        req = ShoppingAssistantRequest(
            message=request.message,
            user_id=request.user_id,
            previous_context=list(request.previous_context),
        )
        result = await ai_service.shopping_assistant(req)

        product_cards = [
            ai_pb2.ProductCardSnippet(
                listing_id=card.listing_id,
                title=card.title,
                price=card.price,
                currency=card.currency,
                image_url=card.image_url,
                discount_rate=card.discount_rate,
                rating_text=card.rating_text,
            )
            for card in result.product_cards
        ]

        return ai_pb2.ShoppingAssistantResponse(
            reply_text=result.reply_text,
            product_cards=product_cards,
            suggested_followups=result.suggested_followups,
        )

    async def MagicListing(
        self,
        request: ai_pb2.MagicListingRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.MagicListingResponse:
        await ensure_scopes(context, *AI_SERVICE_SCOPES["MagicListing"])
        return await self._magic_listing(request, context)

    @map_errors("MagicListing")
    async def _magic_listing(
        self,
        request: ai_pb2.MagicListingRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.MagicListingResponse:
        ai_service = self._ai_provider()
        req = MagicListingRequest(
            title_hint=request.title_hint,
            category_hint=request.category_hint,
            image_url=request.image_url,
        )
        result = await ai_service.magic_listing(req)

        return ai_pb2.MagicListingResponse(
            generated_title=result.generated_title,
            generated_description=result.generated_description,
            suggested_category_id=result.suggested_category_id,
            suggested_price_min=result.suggested_price_min,
            suggested_price_max=result.suggested_price_max,
            highlight_tags=result.highlight_tags,
        )

    async def ChatCopilot(
        self,
        request: ai_pb2.ChatCopilotRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ChatCopilotResponse:
        principal = await ensure_scopes(context, *AI_SERVICE_SCOPES["ChatCopilot"])
        # seller_id is client-supplied: a seller may only act as themselves.
        if (
            request.seller_id
            and request.seller_id != principal.id
            and ADMIN_SCOPE not in principal.scopes
        ):
            await context.abort(
                grpc.StatusCode.PERMISSION_DENIED,
                "insufficient_scope: seller_id does not match the caller",
            )
            raise AssertionError("unreachable")
        return await self._chat_copilot(request, context)

    @map_errors("ChatCopilot")
    async def _chat_copilot(
        self,
        request: ai_pb2.ChatCopilotRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ChatCopilotResponse:
        ai_service = self._ai_provider()
        req = ChatCopilotRequest(
            seller_id=request.seller_id,
            buyer_message=request.buyer_message,
            listing_id=request.listing_id,
        )
        result = await ai_service.chat_copilot(req)

        return ai_pb2.ChatCopilotResponse(
            quick_replies=result.quick_replies,
        )

    async def SummarizeReviews(
        self,
        request: ai_pb2.SummarizeReviewsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.SummarizeReviewsResponse:
        await ensure_scopes(context, *AI_SERVICE_SCOPES["SummarizeReviews"])
        return await self._summarize_reviews(request, context)

    @map_errors("SummarizeReviews")
    async def _summarize_reviews(
        self,
        request: ai_pb2.SummarizeReviewsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.SummarizeReviewsResponse:
        ai_service = self._ai_provider()
        req = SummarizeReviewsRequest(
            listing_id=request.listing_id,
            reviews=[
                ReviewItem(rating=r.rating, comment=r.comment) for r in request.reviews
            ],
        )
        result = await ai_service.summarize_reviews(req)

        return ai_pb2.SummarizeReviewsResponse(
            summary=result.summary,
            pros=result.pros,
            cons=result.cons,
            sentiment=result.sentiment,
        )

    async def ClassifyTags(
        self,
        request: ai_pb2.ClassifyTagsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ClassifyTagsResponse:
        principal = await ensure_scopes(context, *AI_SERVICE_SCOPES["ClassifyTags"])
        # Internal RPC: a user token that somehow carries the scope is still refused.
        if principal.type != "service":
            await context.abort(
                grpc.StatusCode.PERMISSION_DENIED,
                "insufficient_scope: ClassifyTags is for service principals only",
            )
            raise AssertionError("unreachable")
        # Outside map_errors so the abort is never re-mapped.
        if len(request.title.strip()) < 2:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "invalid argument: title"
            )
            raise AssertionError("unreachable")
        return await self._classify_tags(request, context)

    @map_errors("ClassifyTags")
    async def _classify_tags(
        self,
        request: ai_pb2.ClassifyTagsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ClassifyTagsResponse:
        title = request.title.strip()
        service = self._tag_provider()
        if not request.variants:
            spu = await service.classify_tags(
                TagsRequest(
                    title=title,
                    description=request.description,
                    category_id=request.category_id,
                    top_k=30,
                    include_candidates=False,
                )
            )
            return ai_pb2.ClassifyTagsResponse(
                tags=[_tag(t) for t in spu.canonical_tags]
            )
        res = await service.classify_sku_hierarchy(
            ClassifySkuHierarchyRequest(
                spu_title=title,
                spu_description=request.description,
                category_id=request.category_id,
                variants=[
                    SkuVariantInput(
                        variant_id=v.variant_id,
                        name=v.name or v.sku_code or v.variant_id or "variant",
                        sku_code=v.sku_code,
                        price=max(v.price, 0),
                        stock=max(v.stock, 0),
                    )
                    for v in request.variants
                ],
            )
        )
        return ai_pb2.ClassifyTagsResponse(
            tags=[_tag(t) for t in res.spu_canonical_tags],
            skus=[
                ai_pb2.SkuClassification(
                    variant_id=sku.variant_id,
                    tags=[_tag(t) for t in sku.sku_specific_tags],
                )
                for sku in res.sku_results
            ],
        )


def _tag(tag: TagItem) -> ai_pb2.ClassifiedTag:
    return ai_pb2.ClassifiedTag(
        slug=tag.slug,
        name=tag.name,
        facet_group=tag.facet_group.value,
        confidence=tag.confidence,
    )
