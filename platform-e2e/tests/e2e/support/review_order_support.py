"""Shared review fixtures (engagement-review-integrity; the frontend slice imports from here).

A review needs the caller's COMPLETED order, so API-level seeding goes through the real
gateway: place a COD order, the listing's seller ships and completes it, then CreateReview with
that order id. Legacy unverified rows (the API can no longer create them) and bulk rows are
inserted by direct SQL into `engagement_db.reviews` via `docker exec <postgres> psql`.

`buyer` / `seller` are anything with a `.token` (src.models.User or oic Actor); `world` is only
kept for call-site symmetry and is not read.
"""

from __future__ import annotations

import os
import subprocess
import uuid
from typing import Any

import httpx

from config.settings import get_settings

ENGAGEMENT = "/platform.engagement.v1.EngagementService"
ORDER = "/platform.order.v1.OrderService"
CART = "/platform.order.v1.CartService"
ADDRESS = "/platform.identity.v1.AddressService"
AUDIT = "/platform.audit.v1.AuditService"
COMPLETED = "ORDER_STATUS_COMPLETED"
SHIPPED = "ORDER_STATUS_SHIPPED"


def post(token: str | None, service: str, method: str, body: dict | None = None) -> httpx.Response:
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{get_settings().gateway_url}{service}/{method}",
        json=body or {},
        headers=headers,
        timeout=45,
    )


def code_of(resp: httpx.Response) -> str:
    """Connect error code of a failed call (`ok` for a 2xx)."""
    if resp.status_code < 400:
        return "ok"
    try:
        return str(resp.json().get("code", f"http_{resp.status_code}"))
    except ValueError:
        return f"http_{resp.status_code}"


def ok(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"HTTP {resp.status_code}: {resp.text[:400]}"
    return resp.json() if resp.content else {}


# ── orders ───────────────────────────────────────────────────────────────
def pending_order_for(world: Any, buyer: Any, listing_id: str) -> str:
    """Place a one-item COD order for `buyer`; it stays in its initial (non-COMPLETED) status."""
    token = buyer.token
    post(
        token,
        ADDRESS,
        "CreateAddress",
        {
            "recipientName": "Nguyen Van A",
            "phone": "0912345678",
            "street": "29 Lieu Giai",
            "city": "Ha Noi",
            "ward": "Phuong Lieu Giai",
            "district": "Quan Ba Dinh",
            "isDefault": True,
        },
    )  # an existing address is fine; the response is not asserted
    post(token, CART, "ClearCart", {})
    ok(post(token, CART, "AddToCart", {"listingId": listing_id, "quantity": 1}))
    orders = ok(post(token, ORDER, "CreateOrder", {"paymentMethod": "PAYMENT_METHOD_COD"})).get(
        "orders", []
    )
    assert len(orders) == 1, f"expected one order, got {orders}"
    return orders[0]["id"]


def completed_order_for(world: Any, buyer: Any, seller: Any, listing_id: str) -> str:
    """Order of `listing_id` for `buyer`, shipped then completed by the listing's seller."""
    order_id = pending_order_for(world, buyer, listing_id)
    for status in (SHIPPED, COMPLETED):
        ok(post(seller.token, ORDER, "UpdateOrderStatus", {"id": order_id, "status": status}))
    return order_id


# ── reviews through the gateway ──────────────────────────────────────────
def create_review(
    token: str | None,
    listing_id: str,
    rating: int,
    comment: str,
    order_id: str,
    media_urls: list[str] | None = None,
) -> httpx.Response:
    return post(
        token,
        ENGAGEMENT,
        "CreateReview",
        {
            "listingId": listing_id,
            "rating": rating,
            "comment": comment,
            "orderId": order_id,
            "mediaUrls": media_urls or [],
        },
    )


def seed_verified_review(
    world: Any,
    buyer: Any,
    seller: Any,
    listing_id: str,
    rating: int,
    comment: str,
    media_urls: list[str] | None = None,
) -> dict[str, Any]:
    """A verified review backed by a real completed order; returns the review message."""
    order_id = completed_order_for(world, buyer, seller, listing_id)
    review = ok(create_review(buyer.token, listing_id, rating, comment, order_id, media_urls))
    review = review.get("review") or {}
    assert review.get("id"), f"CreateReview returned no review: {review}"
    return review


def list_reviews(listing_id: str, rating: int = 0, page_size: int = 100) -> dict[str, Any]:
    body: dict[str, Any] = {"listingId": listing_id, "page": {"pageSize": page_size}}
    if rating:
        body["ratingFilter"] = rating
    return ok(post(None, ENGAGEMENT, "ListReviews", body))


def listing_summary(listing_id: str) -> dict[str, Any]:
    return ok(post(None, ENGAGEMENT, "GetListingRatingSummary", {"listingId": listing_id}))


# ── direct SQL (engagement_db) ───────────────────────────────────────────
def postgres_container() -> str:
    return os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")


def sql_lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def engagement_psql(sql: str) -> str:
    res = subprocess.run(
        [
            "docker", "exec", postgres_container(), "psql", "-U", "postgres",
            "-d", "engagement_db", "-v", "ON_ERROR_STOP=1", "-At", "-c", sql,
        ],
        capture_output=True,
        text=True,
        timeout=90,
    )  # fmt: skip
    assert res.returncode == 0, f"psql failed: {res.stderr.strip()[:400]}"
    return res.stdout.strip()


def seed_legacy_unverified_review(
    listing_id: str,
    seller_id: str,
    user_id: str,
    rating: int,
    comment: str,
    created_at: str | None = None,
) -> str:
    """Insert a pre-integrity row (`verified_purchase=false`, empty `order_id`); returns its id."""
    review_id = str(uuid.uuid4())
    ts = sql_lit(created_at) if created_at else "now()"
    engagement_psql(
        "INSERT INTO reviews (id, listing_id, seller_id, user_id, rating, comment, "
        "verified_purchase, order_id, created_at) VALUES ("
        f"{sql_lit(review_id)}, {sql_lit(listing_id)}, {sql_lit(seller_id)}, "
        f"{sql_lit(user_id)}, {int(rating)}, {sql_lit(comment)}, false, '', {ts})"
    )
    return review_id


def seed_verified_reviews_sql(
    listing_id: str,
    seller_id: str,
    n: int,
    user_id: str = "",
    comment_prefix: str = "rvp",
) -> list[str]:
    """`n` verified rows with distinct synthetic order ids; comments `rvp-001`.. in creation order.

    created_at advances one second per row so newest-first paging is deterministic.
    """
    user = user_id or f"rvp-user-{uuid.uuid4().hex[:8]}"
    ids = [str(uuid.uuid4()) for _ in range(n)]
    rows = []
    for i, review_id in enumerate(ids, start=1):
        rows.append(
            f"({sql_lit(review_id)}, {sql_lit(listing_id)}, {sql_lit(seller_id)}, "
            f"{sql_lit(user)}, {1 + i % 5}, {sql_lit(f'{comment_prefix}-{i:03d}')}, true, "
            f"{sql_lit('synthetic-order-' + uuid.uuid4().hex)}, "
            f"now() - interval '{n - i} seconds')"
        )
    engagement_psql(
        "INSERT INTO reviews (id, listing_id, seller_id, user_id, rating, comment, "
        "verified_purchase, order_id, created_at) VALUES " + ", ".join(rows)
    )
    return ids
