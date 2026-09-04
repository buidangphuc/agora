"""AIService gRPC servicer — provides ShoppingAssistant, MagicListing, and ChatCopilot.

Adheres to the platform-core gRPC contract (platform.ai.v1.AIService).
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Callable

import grpc
from loguru import logger

from app.modules.business.ai_assistant.schemas import (
    ChatCopilotRequest,
    MagicListingRequest,
    ReviewItem,
    ShoppingAssistantRequest,
    SummarizeReviewsRequest,
)
from app.modules.business.ai_assistant.service import AIAssistantService
from app.transport.grpc._pb.platform.ai.v1 import ai_pb2, ai_pb2_grpc

if TYPE_CHECKING:
    pass

AIProvider = Callable[[], AIAssistantService]


class AIServicer(ai_pb2_grpc.AIServiceServicer):
    def __init__(self, ai_provider: AIProvider | None = None) -> None:
        self._ai_provider = ai_provider or (lambda: AIAssistantService())

    async def ShoppingAssistant(
        self,
        request: ai_pb2.ShoppingAssistantRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ShoppingAssistantResponse:
        try:
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
        except Exception as exc:
            logger.exception("grpc.ShoppingAssistant.error error={}", exc)
            await context.abort(grpc.StatusCode.INTERNAL, str(exc))
            raise

    async def MagicListing(
        self,
        request: ai_pb2.MagicListingRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.MagicListingResponse:
        try:
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
        except Exception as exc:
            logger.exception("grpc.MagicListing.error error={}", exc)
            await context.abort(grpc.StatusCode.INTERNAL, str(exc))
            raise

    async def ChatCopilot(
        self,
        request: ai_pb2.ChatCopilotRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.ChatCopilotResponse:
        try:
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
        except Exception as exc:
            logger.exception("grpc.ChatCopilot.error error={}", exc)
            await context.abort(grpc.StatusCode.INTERNAL, str(exc))
            raise

    async def SummarizeReviews(
        self,
        request: ai_pb2.SummarizeReviewsRequest,
        context: grpc.aio.ServicerContext,
    ) -> ai_pb2.SummarizeReviewsResponse:
        try:
            ai_service = self._ai_provider()
            req = SummarizeReviewsRequest(
                listing_id=request.listing_id,
                reviews=[
                    ReviewItem(rating=r.rating, comment=r.comment)
                    for r in request.reviews
                ],
            )
            result = await ai_service.summarize_reviews(req)

            return ai_pb2.SummarizeReviewsResponse(
                summary=result.summary,
                pros=result.pros,
                cons=result.cons,
                sentiment=result.sentiment,
            )
        except Exception as exc:
            logger.exception("grpc.SummarizeReviews.error error={}", exc)
            await context.abort(grpc.StatusCode.INTERNAL, str(exc))
            raise
