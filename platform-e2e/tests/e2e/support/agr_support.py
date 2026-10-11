"""Helpers for the assistant-rag-grounding scenarios (area agr).

Black box through the gateway (Connect JSON) like `srm_support`/`hrp_support`, which it builds on.
`ShoppingAssistant` is eventually consistent with a publish (listing event -> Kafka -> team-ai indexer ->
embedding through the modelserve router -> RAG store), so every positive observation is a bounded poll.
"""

from __future__ import annotations

import os
from typing import Any

import httpx

from tests.e2e.support import hrp_support as h
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

ASSISTANT = "/platform.ai.v1.AIService/ShoppingAssistant"
INDEX_S = 90.0
# team-ai limits ShoppingAssistant per principal (llm-fake.override.yaml:
# RATE_LIMIT_PRINCIPAL_PER_MINUTE=20 over a 60 s window). One ask every 4 s is 15 a minute, under it.
ASK_EVERY_S = 4.0
RAG_COLLECTION = "rag_documents"


def qdrant_url() -> str:
    return os.environ.get("QDRANT_URL", "http://localhost:6333")


def indexed(listing_id: str) -> bool:
    """True once the listing has points in the RAG collection (the indexer's readiness signal)."""
    resp = httpx.post(
        f"{qdrant_url()}/collections/{RAG_COLLECTION}/points/scroll",
        json={
            "limit": 1,
            "with_payload": False,
            "filter": {"must": [{"key": "document_id", "match": {"value": listing_id}}]},
        },
        timeout=10,
    )
    return resp.status_code == 200 and bool(resp.json()["result"]["points"])


def wait_indexed(world: World, *label: str) -> None:
    """Wait on the real readiness signal, not on assistant answers (those are rate limited)."""
    for name in label:
        listing_id = s.listing(world, name).id
        s.eventually(
            lambda i=listing_id: indexed(i),
            f"listing {listing_id} to be indexed into {RAG_COLLECTION}",
            INDEX_S,
            2.0,
        )


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
    wait_indexed(world, *label)

    def _returned():
        r = ask(world, message)
        if r.status_code == 429:  # over the product's rate limit: not ready, ask again later
            return None
        return want <= set(card_ids(r)) and r

    return s.eventually(
        _returned,
        f"the assistant to return {sorted(want)} for {message!r}",
        INDEX_S,
        ASK_EVERY_S,
    )


def publish(world: World, label: str, title: str) -> s.Listed:
    return h.create(world, label, title, description="agr e2e")
