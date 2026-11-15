"""Helpers for the ai-copilot-grounding scenarios (area ccg).

Black box through the gateway (Connect JSON): the seller creates listings with ListingService and
calls AIService.ChatCopilot with the seller's bearer token. Needs team-ai with COPILOT_BACKEND=mock
(the default), team-domain and team-gateway.
"""

from __future__ import annotations

import re
import time
from typing import Any

import httpx

from tests.e2e.flows.oic_inv_flow import register
from tests.e2e.support import hrp_support as h
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

COPILOT = "/platform.ai.v1.AIService/ChatCopilot"
MAGIC = "/platform.ai.v1.AIService/MagicListing"
REQUEST_ID = re.compile(r"^[0-9a-f]{32}$")
RULES_VERSION = "rules-v1"
LATENCY_CALLS = 200
LATENCY_P95_S = 0.300

AVAILABILITY = re.compile(r"(?<!hết )(?<!không )(?<!chưa )(còn hàng|còn sẵn|có sẵn|luôn có sẵn)", re.I)
OUT_OF_STOCK = re.compile(r"hết hàng|tạm hết|hết sản phẩm|không còn hàng|không còn sẵn", re.I)
DELIVERY_PROMISE = re.compile(
    r"\d+\s*(ngày|giờ|tiếng|h\b|phút)|trong ngày|hỏa tốc|ngay hôm nay|trước\s*\d+\s*h|sau\s*\d+\s*h"
    r"|\d{1,2}\s*(h|giờ)\s*\d{0,2}",
    re.I,
)
ADDRESS_ASK = re.compile(r"địa chỉ", re.I)


def bag(world: World) -> dict[str, Any]:
    return s.ctx(world).extra.setdefault("ccg", {})


def publish(world: World, label: str, *, stock: int, seller=None, price: int = 100_000) -> s.Listed:
    return s.create_listing(
        world, label, stock=stock, title_suffix=f"ccg {label}", price=price, seller=seller
    )


def other_seller(world: World):
    b = bag(world)
    if "seller2" not in b:
        b["seller2"] = register(world, "seller")
    return b["seller2"]


def copilot(world: World, message: str, listing_id: str = "") -> httpx.Response:
    body: dict[str, Any] = {"buyerMessage": message}
    if listing_id:
        body["listingId"] = listing_id
    return s.post(COPILOT, body, s.seller_of(world).token)


def ok(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"{resp.request.url.path} {resp.status_code}: {resp.text[:300]}"
    return resp.json()


def drafts(resp: httpx.Response) -> list[str]:
    return ok(resp).get("quickReplies") or []


def timed_calls(world: World, message: str, listing_id: str, n: int) -> list[float]:
    out: list[float] = []
    for _ in range(n):
        t0 = time.perf_counter()
        resp = copilot(world, message, listing_id)
        out.append(time.perf_counter() - t0)
        assert resp.status_code == 200, f"ChatCopilot {resp.status_code}: {resp.text[:200]}"
    return out


def p95(values: list[float]) -> float:
    ordered = sorted(values)
    return ordered[max(0, int(len(ordered) * 0.95) - 1)]


def magic(world: World) -> httpx.Response:
    return s.post(
        MAGIC,
        {"titleHint": f"{h.word()} áo thun cotton", "categoryHint": "cat-fashion"},
        s.seller_of(world).token,
    )
