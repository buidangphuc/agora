from loguru import logger

from app.modules.business.ai_assistant.schemas import (
    ReviewItem,
    SummarizeReviewsRequest,
    SummarizeReviewsResponse,
)
from app.modules.business.ai_assistant.text import strip_accents

# Aspect keyword map for deterministic pros/cons extraction from review text.
# Each entry maps a Vietnamese aspect label to accent-stripped trigger keywords.
REVIEW_ASPECTS: list[tuple[str, tuple[str, ...]]] = [
    (
        "Chất lượng sản phẩm",
        (
            "chat luong",
            "chat luong tot",
            "ben",
            "chac chan",
            "xin",
            "tot",
            "te",
            "kem",
            "mau hong",
            "hong",
            "loi",
        ),
    ),
    (
        "Giao hàng & đóng gói",
        (
            "giao hang",
            "giao",
            "ship",
            "van chuyen",
            "dong goi",
            "goi",
            "nhanh",
            "cham",
            "lau",
        ),
    ),
    ("Giá cả", ("gia", "gia ca", "re", "dat", "hop ly", "dang tien", "gia tot")),
    (
        "Dịch vụ & hỗ trợ",
        ("dich vu", "shop", "tu van", "nhiet tinh", "phan hoi", "cham soc", "thai do"),
    ),
    (
        "Mẫu mã & thiết kế",
        ("mau ma", "dep", "mau sac", "thiet ke", "form", "kieu dang", "xau"),
    ),
    (
        "Đúng mô tả",
        (
            "dung mo ta",
            "giong hinh",
            "nhu hinh",
            "chuan",
            "sai mo ta",
            "khac hinh",
            "khong giong",
        ),
    ),
]


class ReviewSummarizer:
    def summarize(self, request: SummarizeReviewsRequest) -> SummarizeReviewsResponse:
        reviews = request.reviews
        logger.info(
            "summarize_reviews.generate listing_id={} count={}",
            request.listing_id,
            len(reviews),
        )

        if not reviews:
            return SummarizeReviewsResponse(
                summary="Chưa có đánh giá nào cho sản phẩm này.",
                pros=[],
                cons=[],
                sentiment="unknown",
            )

        rated = [r.rating for r in reviews if 1 <= r.rating <= 5]
        avg = sum(rated) / len(rated) if rated else 0.0
        has_pos = any(r.rating >= 4 for r in reviews)
        has_neg = any(1 <= r.rating <= 2 for r in reviews)

        pros = _extract_review_aspects(reviews, positive=True)
        cons = _extract_review_aspects(reviews, positive=False)
        if has_pos and not pros:
            pros = ["Được khách hàng đánh giá tích cực"]
        if has_neg and not cons:
            cons = ["Một số khách hàng chưa hài lòng"]

        sentiment = _review_sentiment(avg, has_pos=has_pos, has_neg=has_neg)
        summary = _build_review_summary(len(reviews), avg, sentiment, pros, cons)

        return SummarizeReviewsResponse(
            summary=summary,
            pros=pros,
            cons=cons,
            sentiment=sentiment,
        )


def _extract_review_aspects(reviews: list[ReviewItem], *, positive: bool) -> list[str]:
    # Positive aspects come from >=4 star reviews, negatives from <=2 star.
    matched: set[str] = set()
    for review in reviews:
        if positive and review.rating < 4:
            continue
        if not positive and not (1 <= review.rating <= 2):
            continue
        norm = strip_accents(review.comment)
        for label, keywords in REVIEW_ASPECTS:
            if any(kw in norm for kw in keywords):
                matched.add(label)
    # Preserve the stable REVIEW_ASPECTS order for deterministic output.
    return [label for label, _ in REVIEW_ASPECTS if label in matched]


def _review_sentiment(avg: float, *, has_pos: bool, has_neg: bool) -> str:
    if has_pos and has_neg:
        return "mixed"
    if avg >= 3.5:
        return "positive"
    if avg <= 2.5 and avg > 0:
        return "negative"
    return "neutral"


def _build_review_summary(
    count: int,
    avg: float,
    sentiment: str,
    pros: list[str],
    cons: list[str],
) -> str:
    verdict = {
        "positive": "được đánh giá rất tích cực",
        "negative": "nhận nhiều phản hồi chưa hài lòng",
        "mixed": "nhận ý kiến trái chiều",
        "neutral": "được đánh giá ở mức trung bình",
    }.get(sentiment, "được đánh giá ở mức trung bình")

    parts = [
        f"Dựa trên {count} đánh giá (điểm trung bình {avg:.1f}/5), sản phẩm {verdict}."
    ]
    if pros:
        parts.append(f"Điểm được khen nhiều nhất: {pros[0]}.")
    if cons:
        parts.append(f"Điểm cần cải thiện: {cons[0]}.")
    return " ".join(parts)
