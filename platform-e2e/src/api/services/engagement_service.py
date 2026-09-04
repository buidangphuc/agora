"""Favorites, Reviews, Q&A & Disputes service client."""

from __future__ import annotations

from typing import Any

from src.api.services.base_service import BaseService


class EngagementService(BaseService):
    def toggle_favorite(self, listing_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/AddFavorite",
            {"listingId": listing_id},
        )

    def remove_favorite(self, listing_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/RemoveFavorite",
            {"listingId": listing_id},
        )

    def list_favorites(self) -> dict[str, Any]:
        return self.post("/platform.engagement.v1.EngagementService/ListFavorites", {})

    def ask_question(self, listing_id: str, question_text: str) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/AskQuestion",
            {"listingId": listing_id, "questionText": question_text},
        )

    def answer_question(
        self, question_id: str, answer_text: str, is_shop_reply: bool = True
    ) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/AnswerQuestion",
            {"questionId": question_id, "answerText": answer_text, "isShopReply": is_shop_reply},
        )

    def list_questions(self, listing_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/ListQuestionsByListing",
            {"listingId": listing_id},
        )

    def create_dispute(
        self, order_id: str, defendant_id: str, reason: str, evidence_urls: list[str] | None = None
    ) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/CreateDispute",
            {
                "orderId": order_id,
                "defendantId": defendant_id,
                "reason": reason,
                "evidenceUrls": evidence_urls or [],
            },
        )

    def resolve_dispute(
        self, dispute_id: str, status: str = "DISPUTE_STATUS_RESOLVED", resolution: str = ""
    ) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/ResolveDispute",
            {
                "disputeId": dispute_id,
                "status": status,
                "resolution": resolution,
            },
        )

    def add_review(
        self, listing_id: str, rating: int, comment: str = "", order_id: str = ""
    ) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/CreateReview",
            {
                "listingId": listing_id,
                "rating": rating,
                "comment": comment,
                "orderId": order_id,
            },
        )

    def get_rating_summary(self, listing_id: str) -> dict[str, Any]:
        return self.post(
            "/platform.engagement.v1.EngagementService/GetListingRatingSummary",
            {"listingId": listing_id},
        )
