from __future__ import annotations

from pydantic import BaseModel, Field


class ProductCard(BaseModel):
    listing_id: str
    title: str
    price: int
    currency: str = "VND"
    image_url: str = ""
    discount_rate: int = 0
    rating: float = 4.8
    rating_text: str = "4.8"


class ShoppingAssistantRequest(BaseModel):
    message: str = Field(..., description="Buyer query or message")
    user_id: str = Field(default="", description="Optional buyer identifier")
    previous_context: list[str] = Field(
        default_factory=list, description="Recent conversation turns"
    )
    top_k: int = Field(
        default=4, ge=1, le=20, description="Max product cards to return"
    )


class ShoppingAssistantResponse(BaseModel):
    reply_text: str
    product_cards: list[ProductCard] = Field(default_factory=list)
    suggested_followups: list[str] = Field(default_factory=list)


class MagicListingRequest(BaseModel):
    title_hint: str = Field(
        ..., min_length=2, description="Short product name / seller idea"
    )
    category_hint: str = Field(default="", description="Optional category hint")
    image_url: str = Field(default="", description="Optional product image URL")


class MagicListingResponse(BaseModel):
    generated_title: str
    generated_description: str
    suggested_category_id: str
    suggested_price_min: int
    suggested_price_max: int
    highlight_tags: list[str] = Field(default_factory=list)


class ChatCopilotRequest(BaseModel):
    buyer_message: str = Field(
        ..., min_length=1, description="Message received from buyer"
    )
    seller_id: str = Field(default="", description="Optional seller / shop ID")
    listing_id: str = Field(
        default="", description="Optional product listing ID being discussed"
    )


class ChatCopilotResponse(BaseModel):
    quick_replies: list[str] = Field(default_factory=list)


class ReviewItem(BaseModel):
    rating: int = Field(
        default=0, description="Star rating 1-5 (0/out-of-range = unset)"
    )
    comment: str = Field(default="", description="Free-text review body")


class SummarizeReviewsRequest(BaseModel):
    listing_id: str = Field(default="", description="Listing the reviews belong to")
    reviews: list[ReviewItem] = Field(
        default_factory=list, description="Reviews gathered by the caller (stateless)"
    )


class SummarizeReviewsResponse(BaseModel):
    summary: str
    pros: list[str] = Field(default_factory=list)
    cons: list[str] = Field(default_factory=list)
    sentiment: str = "unknown"
