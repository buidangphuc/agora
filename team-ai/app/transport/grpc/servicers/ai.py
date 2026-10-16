"""AIService gRPC servicer (ShoppingAssistant, MagicListing, ChatCopilot, SummarizeReviews, ClassifyTags, ScreenListing).

Access control: every RPC calls ``ensure_scopes`` first, with the scopes from
``app.transport.grpc.scopes``. ChatCopilot also rejects a ``seller_id`` that is not
the caller's own (admin exempt). ClassifyTags is internal: service principals only, with a
title of at least 2 characters. The checks sit outside ``map_errors`` so an abort is
never re-mapped.
"""

from __future__ import annotations

import grpc
from loguru import logger

from app.modules.business.ai_assistant.factory import AIAssistantServices
from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    ReviewItem,
    ShoppingAssistantRequest,
    SummarizeReviewsRequest,
)
from app.modules.business.listing_screen.ports import ScreenService, ScreenVerdict
from app.modules.business.tag_classifier.schemas import (
    ClassifySkuHierarchyRequest,
    ClassifyTagsRequest,
    SkuVariantInput,
    TagItem,
)
from app.modules.business.tag_classifier.service import TagClassifierService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.errors import map_errors
from app.transport.grpc.registry import GrpcServiceRegistration
from app.transport.grpc.scopes import ADMIN_SCOPE, AI_SERVICE_SCOPES, ai_use_scopes

_SERVICE_NAME = "platform.ai.v1.AIService"


class AIServicer(ai_pb2_grpc.AIServiceServicer):
    def __init__(
        self,
        services: AIAssistantServices,
        tags: TagClassifierService,
        screen: ScreenService,
        *,
        require_ai_use: bool,
    ) -> None:
        self._services = services
        self._tags = tags
        self._screen = screen
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
        result = await self._services.shopping.answer(
            ShoppingAssistantRequest(
                message=request.message,
                user_id=request.user_id,
                previous_context=list(request.previous_context),
            )
        )
        return ai_pb2.ShoppingAssistantResponse(
            reply_text=result.reply_text,
            product_cards=[
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
            ],
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
        result = self._services.listing.generate(
            MagicListingRequest(
                title_hint=request.title_hint,
                category_hint=request.category_hint,
                image_url=request.image_url,
            )
        )
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
            raise AssertionError("unreachable")  # abort raises
        return await self._chat_copilot(request, context)

    @map_errors("ChatCopilot")
    async def _chat_copilot(
        self,
        request: ai_pb2.ChatCopilotRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ChatCopilotResponse:
        result = self._services.copilot.suggest(
            ChatCopilotRequest(
                seller_id=request.seller_id,
                buyer_message=request.buyer_message,
                listing_id=request.listing_id,
            )
        )
        return ai_pb2.ChatCopilotResponse(quick_replies=result.quick_replies)

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
        result = self._services.reviews.summarize(
            SummarizeReviewsRequest(
                listing_id=request.listing_id,
                reviews=[
                    ReviewItem(rating=r.rating, comment=r.comment)
                    for r in request.reviews
                ],
            )
        )
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
            raise AssertionError("unreachable")  # abort raises
        if len(request.title.strip()) < 2:
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "invalid argument: title"
            )
            raise AssertionError("unreachable")  # abort raises
        return await self._classify_tags(request, context)

    @map_errors("ClassifyTags")
    async def _classify_tags(
        self,
        request: ai_pb2.ClassifyTagsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ClassifyTagsResponse:
        title = request.title.strip()
        if not request.variants:
            spu = self._tags.classify_tags(
                ClassifyTagsRequest(
                    title=title,
                    description=request.description,
                    category_id=request.category_id,
                    top_k=30,
                    include_candidates=False,
                )
            )
            return ai_pb2.ClassifyTagsResponse(
                tags=[_classified(t) for t in spu.canonical_tags]
            )
        result = self._tags.classify_sku_hierarchy(
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
            tags=[_classified(t) for t in result.spu_canonical_tags],
            skus=[
                ai_pb2.SkuClassification(
                    variant_id=sku.variant_id,
                    tags=[_classified(t) for t in sku.sku_specific_tags],
                )
                for sku in result.sku_results
            ],
        )

    async def ScreenListing(
        self,
        request: ai_pb2.ScreenListingRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ScreenListingResponse:
        principal = await ensure_scopes(context, *AI_SERVICE_SCOPES["ScreenListing"])
        # Internal RPC: a user token that somehow carries the scope is still refused.
        if principal.type != "service":
            await context.abort(
                grpc.StatusCode.PERMISSION_DENIED,
                "insufficient_scope: ScreenListing is for service principals only",
            )
            raise AssertionError("unreachable")  # abort raises
        if not request.title.strip():
            await context.abort(
                grpc.StatusCode.INVALID_ARGUMENT, "invalid argument: title"
            )
            raise AssertionError("unreachable")  # abort raises
        return await self._screen_listing(request, context)

    @map_errors("ScreenListing")
    async def _screen_listing(
        self,
        request: ai_pb2.ScreenListingRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ScreenListingResponse:
        result = await self._screen.screen(
            title=request.title,
            description=request.description,
            listing_id=request.listing_id,
            category_id=request.category_id,
        )
        # Never log listing text: id, verdict and backend only.
        logger.info(
            "screen_listing listing_id={} verdict={} backend={}",
            request.listing_id,
            result.verdict.value,
            result.backend,
        )
        verdict = (
            ai_pb2.SCREEN_VERDICT_REVIEW
            if result.verdict is ScreenVerdict.REVIEW
            else ai_pb2.SCREEN_VERDICT_ALLOW
        )
        return ai_pb2.ScreenListingResponse(
            verdict=verdict, labels=list(result.labels), backend=result.backend
        )


def _classified(tag: TagItem) -> ai_pb2.ClassifiedTag:
    return ai_pb2.ClassifiedTag(
        slug=tag.slug,
        name=tag.name,
        facet_group=tag.facet_group.value,
        confidence=tag.confidence,
    )


def ai_service_registration(
    services: AIAssistantServices,
    tags: TagClassifierService,
    screen: ScreenService,
    *,
    require_ai_use: bool,
) -> GrpcServiceRegistration:
    servicer = AIServicer(services, tags, screen, require_ai_use=require_ai_use)
    return GrpcServiceRegistration(
        _SERVICE_NAME,
        lambda server: ai_pb2_grpc.add_AIServiceServicer_to_server(servicer, server),
        frozenset({f"/{_SERVICE_NAME}/ShoppingAssistant"}),
    )
