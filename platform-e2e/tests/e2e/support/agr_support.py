"""Helpers for the assistant-rag-grounding scenarios (area agr).

Black box through the gateway (Connect JSON) like `srm_support`/`hrp_support`, which it builds on.
`ShoppingAssistant` is eventually consistent with a publish (listing event -> Kafka -> team-ai indexer ->
embedding through the modelserve router -> RAG store), so every positive observation is a bounded poll.
"""

from __future__ import annotations

from typing import Any

import httpx

from tests.e2e.support import hrp_support as h
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

ASSISTANT = "/platform.ai.v1.AIService/ShoppingAssistant"
INDEX_S = 90.0


def bag(world: World) -> dict[str, Any]:
    return s.ctx(world).extra.setdefault("agr", {})


def ask(world: World, message: str) -> httpx.Response:
    return s.post(ASSISTANT, {"message": message}, s.buyer_of(world).token)


def cards(resp: httpx.Response) -> list[dict[str, Any]]:
    assert resp.status_code == 200, f"ShoppingAssistant {resp.status_code}: {resp.text[:300]}"
    return resp.json().get("productCards") or []


def card_ids(resp: httpx.Response) -> list[str]:
    return [c.get("listingId", "") for c in cards(resp)]


def wait_returned(world: World, message: str, *label: str) -> httpx.Response:
    """Positive control: the assistant returns every listing in `label` for `message`."""
    want = {s.listing(world, name).id for name in label}
    return s.eventually(
        lambda: (r := ask(world, message)) and want <= set(card_ids(r)) and r,
        f"the assistant to return {sorted(want)} for {message!r}",
        INDEX_S,
        2.0,
    )


def publish(world: World, label: str, title: str) -> s.Listed:
    return h.create(world, label, title, description="agr e2e")
