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
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc
from app.transport.grpc.context import ensure_scopes
from app.transport.grpc.errors import map_errors
from app.transport.grpc.scopes import ADMIN_SCOPE, AI_SERVICE_SCOPES, ai_use_scopes

AIProvider = Callable[[], AIAssistantService]


class AIServicer(ai_pb2_grpc.AIServiceServicer):
    def __init__(
        self,
        ai_provider: AIProvider | None = None,
        *,
        require_ai_use: bool = False,
    ) -> None:
        self._ai_provider = ai_provider or (lambda: AIAssistantService())
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
