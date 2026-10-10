"""Helpers for the add-hybrid-retrieval-platform scenarios (area hrp).

Black box through the gateway (Connect JSON) like `srm_support`, which it builds on for actors,
polling and the Kafka / OpenSearch readers. The model server behind team-search is the
platform-modelserve router with the deterministic TEI fake (compose overlay
`platform-e2e/compose/modelserve.override.yaml`); see `ms_support` for how the fake's request log
proves "an embedding call was / was not made".

Semantics of the fake that these scenarios rely on (platform-e2e/fakes/tei_fake/server.py):

* a text is a hashed bag of concepts, so a title and a query that share a word are close;
* a word `<stem>zalias` is the same concept as `<stem>`: a per-run unique pair that is
  lexically different (BM25 finds nothing) yet semantically identical (k-NN finds it);
* `[[fake status=500]]`, `[[fake delay=4000]]`, `[[fake rerank=reverse]]`,
  `[[fake rerank_status=500]]` inside a query or a title drive the failure scenarios, and are
  removed before hashing.
"""

from __future__ import annotations

import time
import uuid
from typing import Any

import httpx

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.support import ms_support as ms
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

DEFAULT_DESCRIPTION = "hrp"
FUSION_WINDOW = 200  # HYBRID_FUSION_WINDOW default; the overlay does not change it
HYBRID = "SEARCH_MODE_HYBRID"
LEXICAL = "SEARCH_MODE_LEXICAL"
SEMANTIC = "SEARCH_MODE_SEMANTIC"


def bag(world: World) -> dict[str, Any]:
    """Per-scenario scratch dict (hits, timings, names) next to the srm context."""
    return s.ctx(world).extra.setdefault("hrp", {})


def word() -> str:
    """A unique word nothing else in the index contains."""
    return "zhr" + uuid.uuid4().hex[:10]


def create(
    world: World,
    label: str,
    title: str,
    *,
    description: str = DEFAULT_DESCRIPTION,
    price: int = 100_000,
    stock: int = 5,
) -> s.Listed:
    resp = s.post(
        s.LISTING + "CreateListing",
        {
            "listing": {
                "title": title,
                "categoryId": "cat-laptop",
                "price": price,
                "stock": stock,
                "status": "LISTING_STATUS_PUBLISHED",
                "currency": "VND",
                "description": description,
            }
        },
        s.seller_of(world).token,
    )
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    listed = s.Listed(resp.json()["listing"]["id"], title)
    s.ctx(world).listings[label] = listed
    return listed


def search(
    query: str,
    *,
    mode: str | None = None,
    cursor: str | None = None,
    page_size: int | None = None,
    filters: dict[str, str] | None = None,
    min_price: int | None = None,
    max_price: int | None = None,
) -> httpx.Response:
    body: dict[str, Any] = {"query": query}
    if min_price:
        body["minPrice"] = min_price
    if max_price:
        body["maxPrice"] = max_price
    if mode:
        body["searchMode"] = mode
    if filters:
        body["filters"] = filters
    page: dict[str, Any] = {}
    if cursor is not None:
        page["cursor"] = cursor
    if page_size:
        page["pageSize"] = page_size
    if page:
        body["page"] = page
    return s.post(s.SEARCH + "SearchListings", body)


def ids(resp: httpx.Response) -> list[str]:
    return s.hit_ids(resp)


def wait_searchable(world: World, label: str, query: str) -> None:
    """Positive control: a lexical search for `query` returns the listing (the indexer caught up)."""
    lid = s.listing(world, label).id
    s.eventually(
        lambda: lid in ids(search(query, mode=LEXICAL)), f"{label} {lid} lexically searchable", 90.0
    )


def wait_embedded(world: World, label: str) -> dict[str, Any]:
    """Positive control: the read-model document carries a 384 dimension embedding."""
    lid = s.listing(world, label).id

    def _doc() -> dict[str, Any] | None:
        doc = ev.os_doc(lid)
        return doc if doc and len(doc.get("embedding") or []) == ms.EMBED_DIM else None

    return s.eventually(_doc, f"{label} {lid} to be embedded in the read-model", 90.0)


def embed_calls(text: str) -> list[dict[str, Any]]:
    """Embedding requests the TEI fake received whose body contains `text`."""
    return ms.fake_calls(text, "embed")


def rerank_calls(text: str) -> list[dict[str, Any]]:
    return ms.fake_calls(text, "rerank")


def timed(call) -> tuple[Any, float]:  # noqa: ANN001
    started = time.monotonic()
    value = call()
    return value, time.monotonic() - started
