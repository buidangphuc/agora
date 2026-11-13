"""Helpers for the ai-assistant-grounding scenarios (area agr).

Black box through the gateway (Connect JSON) like `srm_support`/`hrp_support`, which it builds on.
`ShoppingAssistant` reads team-search (hydrated from team-domain), so it is eventually consistent with a
publish (listing event -> Kafka -> team-search indexer -> OpenSearch). Every positive observation is a
bounded poll; readiness is the real search signal, not the assistant's rate-limited answers.
"""

from __future__ import annotations

import re
from typing import Any

import httpx

from tests.e2e.support import hrp_support as h
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

ASSISTANT = "/platform.ai.v1.AIService/ShoppingAssistant"
INDEX_S = 90.0
ASK_EVERY_S = 4.0
REQUEST_ID = re.compile(r"^[0-9a-f]{32}$")
MODEL_VERSION = re.compile(r"^mock:assistant-v1\|.+$")
RETRIEVAL_FAILED_MODEL_VERSION = "mock:assistant-v1"


def bag(world: World) -> dict[str, Any]:
    return s.ctx(world).extra.setdefault("agr", {})


def word(world: World) -> str:
    return bag(world).setdefault("word", h.word())


def ask(world: World, message: str) -> httpx.Response:
    return s.post(ASSISTANT, {"message": message}, s.buyer_of(world).token)


def body(resp: httpx.Response) -> dict[str, Any]:
    assert resp.status_code == 200, f"ShoppingAssistant {resp.status_code}: {resp.text[:300]}"
    return resp.json()


def cards(resp: httpx.Response) -> list[dict[str, Any]]:
    return body(resp).get("productCards") or []


def card_ids(resp: httpx.Response) -> list[str]:
    return [c.get("listingId", "") for c in cards(resp)]


def publish(
    world: World, label: str, title: str, *, price: int = 100_000, stock: int = 5
) -> s.Listed:
    return h.create(world, label, title, description="agr e2e", price=price, stock=stock)


def domain_listing(listing_id: str) -> dict[str, Any]:
    return s.ok_json(s.post(s.LISTING + "GetListing", {"id": listing_id}))["listing"]


def wait_searchable(world: World, *label: str, strict: bool = True) -> None:
    for name in label:
        try:
            s.wait_indexed(world, name)
        except AssertionError:
            if strict:
                raise


def wait_returned(world: World, message: str, *label: str) -> httpx.Response:
    """Positive control: the assistant returns every listing in `label` for `message`."""
    want = {s.listing(world, name).id for name in label}
    wait_searchable(world, *label)

    def _returned():
        r = ask(world, message)
        if r.status_code == 429:
            return None
        return want <= set(card_ids(r)) and r

    return s.eventually(
        _returned,
        f"the assistant to return {sorted(want)} for {message!r}",
        INDEX_S,
        ASK_EVERY_S,
    )


def digits(text: str) -> str:
    return re.sub(r"\D", "", text)
