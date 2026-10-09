"""Steps for search/hybrid_retrieval.feature and search/hybrid_indexing.feature
(change add-hybrid-retrieval-platform, spec search-retrieval).

Everything the buyer sees goes through the gateway; the model server is the platform-modelserve
router in front of the deterministic TEI fake (`compose/modelserve.override.yaml`), and the fake's
request log is where "an embedding was / was not requested" is observed. Vectors and the
`vector_pending` flag are read from the OpenSearch read-model document, the way the tombstone
scenarios read theirs.
"""

from __future__ import annotations

import os
import subprocess
import time
import uuid

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.flows.stack_flow import stop_container
from tests.e2e.support import hrp_support as h
from tests.e2e.support import ms_support as ms
from tests.e2e.support import srm_support as s
from tests.e2e.support.world import World

LISTING_L = "L"


def _kw(world: World) -> str:
    return s.ctx(world).keyword


def _sorted_ours(world: World, resp: httpx.Response, labels: list[str]) -> list[str]:
    mine = {s.listing(world, label).id for label in labels}
    return [i for i in h.ids(resp) if i in mine]


# ── givens: listings ─────────────────────────────────────────────────────
@given("a published listing whose title carries a unique keyword is searchable")
def keyword_listing(world: World) -> None:
    h.create(world, LISTING_L, f"{_kw(world)} fixture")
    h.wait_searchable(world, LISTING_L, _kw(world))


@given("a published listing with a unique keyword is embedded and searchable")
def keyword_listing_embedded(world: World) -> None:
    h.create(world, LISTING_L, f"{_kw(world)} fixture")
    h.wait_searchable(world, LISTING_L, _kw(world))
    h.wait_embedded(world, LISTING_L)


@given("an embedded listing titled with a unique word")
def word_listing(world: World) -> None:
    stem = h.word()
    h.bag(world)["stem"] = stem
    h.create(world, "near", f"{stem} fixture")
    h.wait_embedded(world, "near")


@given("an embedded listing titled with a unique word and an embedded unrelated listing")
def word_and_unrelated(world: World) -> None:
    word_listing(world)
    h.create(world, "far", f"{h.word()} blender")
    h.wait_embedded(world, "far")


@given("two embedded listings with a unique keyword differ in price")
def two_priced(world: World) -> None:
    kw = _kw(world)
    h.create(world, "cheap", f"{kw} cheap", price=100_000)
    h.create(world, "dear", f"{kw} dear", price=600_000)
    for label in ("cheap", "dear"):
        h.wait_searchable(world, label, kw)
        h.wait_embedded(world, label)


@given("three embedded listings share a unique keyword")
def three_listings(world: World) -> None:
    kw = _kw(world)
    for label in ("one", "two", "three"):
        h.create(world, label, f"{kw} {label}")
    for label in ("one", "two", "three"):
        h.wait_searchable(world, label, kw)
        h.wait_embedded(world, label)


@given(
    "an embedded listing matching a keyword lexically and an embedded listing matching its alias "
    "only semantically"
)
def lexical_and_semantic_only(world: World) -> None:
    stem = h.word()
    h.bag(world)["stem"] = stem
    h.create(world, "lex", f"{_kw(world)} lexical")
    h.create(world, "sem", f"{stem} fixture")
    h.wait_searchable(world, "lex", _kw(world))
    h.wait_embedded(world, "lex")
    h.wait_embedded(world, "sem")


@given("the modelserve router is stopped")
def stop_router(world: World) -> None:
    restore = stop_container(ms.router_container())

    def _restart() -> None:
        restore()
        s.eventually(
            lambda: httpx.get(ms.router_url() + "/healthz", timeout=3).status_code == 200,
            "the modelserve router to come back",
            90.0,
            2.0,
        )

    world.add_cleanup(_restart)


# ── whens: searches ──────────────────────────────────────────────────────
@when("a buyer searches for that keyword in SEARCH_MODE_LEXICAL through the gateway")
def search_keyword_lexical(world: World) -> None:
    h.bag(world)["resp"] = h.search(_kw(world), mode=h.LEXICAL)


@when("a buyer searches for that keyword in SEARCH_MODE_HYBRID through the gateway")
def search_keyword_hybrid(world: World) -> None:
    h.bag(world)["resp"] = h.search(_kw(world), mode=h.HYBRID)


def _alias(world: World) -> str:
    alias = f"{h.bag(world)['stem']}zalias"
    h.bag(world)["query"] = alias
    return alias


def _search_until_listed(world: World, query: str, mode: str | None, label: str) -> None:
    lid = s.listing(world, label).id
    h.bag(world)["resp"] = s.eventually(
        lambda: (r := h.search(query, mode=mode)) and lid in h.ids(r) and r,
        f"{label} {lid} among the hits of {query!r} in mode {mode}",
        30.0,
    )


@when("a buyer searches for the semantic alias of that word in SEARCH_MODE_SEMANTIC")
def search_alias_semantic(world: World) -> None:
    _search_until_listed(world, _alias(world), h.SEMANTIC, "near")


@when("a buyer searches for the semantic alias of that word without a search mode")
def search_alias_unspecified(world: World) -> None:
    _search_until_listed(world, _alias(world), None, "near")


@when("a buyer searches for that keyword plus a slow-embedding directive in SEARCH_MODE_HYBRID")
def search_slow(world: World) -> None:
    query = f"{_kw(world)} [[fake delay=4000]]"
    h.bag(world)["query"] = query
    resp, took = h.timed(lambda: h.search(query, mode=h.HYBRID))
    h.bag(world).update(resp=resp, took=took)


@when("a buyer searches for that keyword plus a failing-embedding directive in SEARCH_MODE_HYBRID")
def search_failing(world: World) -> None:
    query = f"{_kw(world)} [[fake status=500]]"
    h.bag(world).update(
        query=query,
        resp=h.search(query, mode=h.HYBRID),
        lexical=h.search(query, mode=h.LEXICAL),
    )


@when(
    "a buyer searches for that keyword plus a unique word in SEARCH_MODE_LEXICAL through the gateway"
)
def search_unique_lexical(world: World) -> None:
    query = f"{_kw(world)} {h.word()}"
    h.bag(world).update(query=query, resp=h.search(query, mode=h.LEXICAL))


@when(
    "a buyer searches for that keyword in SEARCH_MODE_HYBRID, then again with a reverse-rerank "
    "directive"
)
def search_reverse_rerank(world: World) -> None:
    _plain_then_directive(world, "[[fake rerank=reverse]]")


@when(
    "a buyer searches for that keyword in SEARCH_MODE_HYBRID, then again with a failing-rerank "
    "directive"
)
def search_failing_rerank(world: World) -> None:
    _plain_then_directive(world, "[[fake rerank_status=500]]")


def _plain_then_directive(world: World, directive: str) -> None:
    labels = ["one", "two", "three"]
    kw = _kw(world)

    def _plain():
        resp = h.search(kw, mode=h.HYBRID)
        return resp if len(_sorted_ours(world, resp, labels)) == 3 else None

    plain = s.eventually(_plain, "all three listings in the plain hybrid search", 30.0)
    directed = h.search(f"{kw} {directive}", mode=h.HYBRID)
    h.bag(world).update(
        plain=_sorted_ours(world, plain, labels),
        directed=_sorted_ours(world, directed, labels),
        directed_resp=directed,
        query=kw,
    )


@when("a buyer searches for the keyword and the alias in SEARCH_MODE_HYBRID, first page")
def search_keyword_and_alias(world: World) -> None:
    alias = f"{h.bag(world)['stem']}zalias"
    query = f"{_kw(world)} {alias}"
    sem = s.listing(world, "sem").id
    h.bag(world).update(
        query=query,
        alias=alias,
        resp=s.eventually(
            lambda: (r := h.search(query, mode=h.HYBRID)) and sem in h.ids(r) and r,
            "the semantic-only listing in the hybrid hits",
            30.0,
        ),
        lexical=h.search(query, mode=h.LEXICAL),
    )


@when(
    "a buyer searches for that keyword plus a unique word in SEARCH_MODE_HYBRID with the cursor 250"
)
def search_deep(world: World) -> None:
    query = f"{_kw(world)} {h.word()}"
    h.bag(world).update(
        query=query,
        resp=h.search(query, mode=h.HYBRID, cursor="250"),
        lexical=h.search(query, mode=h.LEXICAL, cursor="250"),
    )


# ── thens: query side ────────────────────────────────────────────────────
@then("the listing is among the hits with a positive score")
def positive_score(world: World) -> None:
    hit = s.hit_of(h.bag(world)["resp"], s.listing(world, LISTING_L).id)
    assert hit is not None, "the listing is not among the lexical hits"
    assert float(hit.get("score", 0)) > 0, hit


@then(
    "the listing with that word is the first hit and the unrelated one ranks after it or is absent"
)
def near_first(world: World) -> None:
    got = h.ids(h.bag(world)["resp"])
    near, far = s.listing(world, "near").id, s.listing(world, "far").id
    assert got and got[0] == near, f"first hit {got[:1]} is not the nearest listing {near}"
    assert far not in got or got.index(far) > 0


@then("a lexical search for the same alias finds neither listing")
def lexical_finds_none(world: World) -> None:
    got = set(h.ids(h.search(h.bag(world)["query"], mode=h.LEXICAL)))
    mine = {s.listing(world, label).id for label in ("near", "far")}
    assert not got & mine, f"BM25 matched {got & mine}: the alias is not lexically distinct"


@then("the search answers 200 with the listing among the hits")
def ok_with_listing(world: World) -> None:
    resp = h.bag(world)["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    assert s.listing(world, LISTING_L).id in h.ids(resp)


@then(
    "the search answers 200 with the listing among the hits well before the embedding would finish"
)
def fast_fallback(world: World) -> None:
    ok_with_listing(world)
    took = h.bag(world)["took"]
    assert took < 3.5, f"the search took {took:.1f}s: it waited on the 4s embedding"


@then("the TEI fake did receive the slow embedding request")
def slow_seen(world: World) -> None:
    query = h.bag(world)["query"]
    s.eventually(lambda: h.embed_calls(query), "the slow embedding request at the TEI fake", 15.0)


@then("the TEI fake rejected the embedding of that query")
def embed_rejected(world: World) -> None:
    query = h.bag(world)["query"]
    calls = s.eventually(
        lambda: h.embed_calls(query), "the embedding request at the TEI fake", 15.0
    )
    assert all(c["status"] == 500 for c in calls), [c["status"] for c in calls]


def _bucket_counts(facets: dict, name: str) -> dict[str, int]:
    return {b["key"]: int(b["count"]) for b in facets.get(name) or []}


@then(
    "the facet counts of the degraded response equal those of the lexical search and count both "
    "listings"
)
def facets_kept(world: World) -> None:
    bag = h.bag(world)
    degraded, lexical = s.ok_json(bag["resp"]), s.ok_json(bag["lexical"])
    assert set(h.ids(bag["resp"])) >= {s.listing(world, x).id for x in ("cheap", "dear")}
    facets = degraded.get("facets") or {}
    assert facets == (lexical.get("facets") or {}), "facets differ between degraded and lexical"
    assert _bucket_counts(facets, "categories") == {"cat-laptop": 2}, facets.get("categories")
    prices = {k: v for k, v in _bucket_counts(facets, "priceRanges").items() if v}
    assert len(prices) >= 2 and sum(prices.values()) == 2, prices
    assert _bucket_counts(facets, "sellers") == {s.subject(s.seller_of(world).token): 2}


@then("the listing is among the hits although a lexical search for the alias finds nothing")
def listed_only_semantically(world: World) -> None:
    near = s.listing(world, "near").id
    assert near in h.ids(h.bag(world)["resp"])
    assert near not in h.ids(h.search(h.bag(world)["query"], mode=h.LEXICAL))


@then("the TEI fake received an embedding request for that query")
def embed_seen(world: World) -> None:
    query = h.bag(world)["query"]
    assert h.embed_calls(query), f"the TEI fake never saw an embedding of {query!r}"


@then("the listing is among the hits")
def listing_among(world: World) -> None:
    assert s.listing(world, LISTING_L).id in h.ids(h.bag(world)["resp"])


@then("the TEI fake received no embedding request for that query")
def embed_not_seen(world: World) -> None:
    query = h.bag(world)["query"]
    assert query.split()[-1] and not h.embed_calls(
        query.split()[-1]
    ), f"an embedding of {query!r} reached the TEI fake"


@then("the same query in SEARCH_MODE_HYBRID does reach the TEI fake")
@then("the same query in SEARCH_MODE_HYBRID at the first page does reach the TEI fake")
def hybrid_reaches_fake(world: World) -> None:
    query = h.bag(world)["query"]
    assert h.search(query, mode=h.HYBRID).status_code == 200
    s.eventually(
        lambda: h.embed_calls(query.split()[-1]), f"an embedding of {query!r} at the TEI fake", 15.0
    )


@then("the listing is the first hit")
def listing_first(world: World) -> None:
    got = h.ids(h.bag(world)["resp"])
    assert got[:1] == [s.listing(world, "near").id], got[:3]


@then("the TEI fake received a rerank request listing those candidates")
def rerank_seen(world: World) -> None:
    mine = {s.listing(world, label).id for label in ("one", "two", "three")}

    def _found():
        for call in h.rerank_calls('"texts"'):
            if all(i in call["body"] for i in mine):
                return call
        return None

    s.eventually(_found, "a rerank request carrying the three candidate ids", 15.0)


@then("the three listings come back in the opposite order of the plain search")
def reversed_order(world: World) -> None:
    bag = h.bag(world)
    assert len(bag["plain"]) == 3 and bag["directed"] == list(
        reversed(bag["plain"])
    ), f"plain {bag['plain']} directed {bag['directed']}: the reranker order was not applied"


@then("the TEI fake answered the rerank request with a failure")
def rerank_failed(world: World) -> None:
    def _failed():
        return [c for c in h.rerank_calls("rerank_status=500") if c["status"] == 500]

    s.eventually(_failed, "a failed rerank request at the TEI fake", 15.0)


@then("the search answers 200 with the three listings in the same order as the plain search")
def same_order(world: World) -> None:
    bag = h.bag(world)
    assert bag["directed_resp"].status_code == 200
    assert len(bag["plain"]) == 3 and bag["directed"] == bag["plain"]


@then("both listings are among the hits although a lexical search finds only the first one")
def fused_both(world: World) -> None:
    bag = h.bag(world)
    lex, sem = s.listing(world, "lex").id, s.listing(world, "sem").id
    assert {lex, sem} <= set(h.ids(bag["resp"])), h.ids(bag["resp"])
    assert lex in h.ids(bag["lexical"]) and sem not in h.ids(bag["lexical"])
    bag["query"] = bag["alias"]


@then("the answer is 200 with the same total as the lexical search at that cursor")
def same_total(world: World) -> None:
    bag = h.bag(world)
    assert bag["resp"].status_code == 200, bag["resp"].text[:300]
    assert s.total_of(bag["resp"]) == s.total_of(bag["lexical"]) >= 1


# ── indexing: givens / whens ─────────────────────────────────────────────
@given("a published listing whose read-model document is embedded")
def embedded_doc(world: World) -> None:
    h.create(world, LISTING_L, f"{_kw(world)} indexed")
    h.bag(world)["doc"] = h.wait_embedded(world, LISTING_L)


@when("a seller publishes a listing with a unique title through the gateway")
def publish_unique(world: World) -> None:
    h.create(world, LISTING_L, f"{_kw(world)} indexed")


@when("a seller publishes a listing whose title carries a failing-embedding directive")
def publish_failing(world: World) -> None:
    h.create(world, LISTING_L, f"{_kw(world)} [[fake status=500]]")


@when(
    "a ListingPricingChanged with a new price and a newer occurred_at is published to listing.events"
)
def publish_price(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    h.bag(world)["new_price"] = 777_000
    ev.publish_and_wait(lid, ev.pricing_changed(lid, 777_000, ev.now_ns()), s.CONSUME_S)


@when(
    "a ListingChanged UPDATED with another title and an occurred_at older than the creation is "
    "published to listing.events"
)
def publish_stale(world: World) -> None:
    created = s.real_event(world, LISTING_L, ev.LISTING_CHANGED, ev.CREATED)
    stale_title = f"{h.word()} stale"
    h.bag(world)["stale_title"] = stale_title
    value = ev.listing_changed(
        ev.UPDATED,
        s.craft_listing(
            world, LISTING_L, title=stale_title, description=h.DEFAULT_DESCRIPTION, stock=5
        ),
        created.occurred_at_ns - 5_000_000_000,
    )
    ev.publish_and_wait(s.listing(world, LISTING_L).id, value, s.CONSUME_S)


# ── indexing: thens ──────────────────────────────────────────────────────
@then(
    "the read-model document holds a 384 dimension embedding that is the model server's vector of "
    "its text"
)
def doc_embedded(world: World) -> None:
    doc = h.wait_embedded(world, LISTING_L)
    text = f"{doc['title']} {doc.get('description', '')}".strip()
    expected = ms.fake_vectors([text])[0]
    got = doc["embedding"]
    assert len(got) == ms.EMBED_DIM
    assert max(abs(a - b) for a, b in zip(got, expected, strict=True)) < 1e-4
    h.bag(world)["doc"] = doc


@then("the document is not marked vector_pending")
def not_pending(world: World) -> None:
    assert not h.bag(world)["doc"].get("vector_pending")


@then("the listing is indexed and searchable with vector_pending true and no embedding")
def pending_doc(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    h.wait_searchable(world, LISTING_L, _kw(world))
    doc = ev.os_doc(lid)
    assert doc is not None
    assert doc.get("vector_pending") is True, doc.get("vector_pending")
    assert not doc.get("embedding"), "a failed embedding left a vector in the document"


@then("no record of the listing is parked on the listing events DLQ")
def no_dlq(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    assert ev.dlq_records_for(lid, s.ctx(world).started_ms) == []


@then("the document shows the new price and the same embedding, still not vector_pending")
def price_kept_embedding(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    bag = h.bag(world)
    doc = s.eventually(
        lambda: (d := ev.os_doc(lid)) and d.get("price") == bag["new_price"] and d,
        "the new price in the read-model",
        60.0,
    )
    assert doc["embedding"] == bag["doc"]["embedding"]
    assert not doc.get("vector_pending")


@then("the read-model document keeps its title and embedding")
def stale_ignored(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    bag = h.bag(world)
    doc = ev.os_doc(lid)
    assert doc["title"] == s.listing(world, LISTING_L).title, doc["title"]
    assert doc["embedding"] == bag["doc"]["embedding"]
    assert lid not in h.ids(h.search(bag["stale_title"].split()[0], mode=h.LEXICAL))


# ── replay onto a new index (destructive: a second indexer, a full topic read) ──
def _docker(*args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(["docker", *args], capture_output=True, text=True, timeout=90, check=True)


@when("a second indexer with a new consumer group replays listing.events onto a new index")
def replay(world: World) -> None:
    tag = uuid.uuid4().hex[:8]
    index, name = f"hrp-replay-{tag}", f"hrp-replay-{tag}"
    h.bag(world)["replay_index"] = index
    network = os.getenv("STACK_NETWORK", "agora_default")
    image = os.getenv("TS_IMAGE", "agora-team-search:local")
    env = {
        "ENV": "local",
        "KAFKA_ENABLED": "true",
        "KAFKA_BROKERS": os.getenv("HRP_KAFKA_INTERNAL", "redpanda:9092"),
        "KAFKA_CONSUMER_GROUP": f"hrp-replay-{tag}",
        "OPENSEARCH_URL": os.getenv("HRP_OPENSEARCH_INTERNAL", "http://opensearch:9200"),
        "OPENSEARCH_INDEX": index,
        "MODEL_SERVER_URL": "http://modelserve-router:8100",
        "UPSTREAM_AI_ADDR": os.getenv("HRP_AI_INTERNAL", "team-ai-svc:50060"),
        "DATABASE_ENABLED": "false",
    }
    args = ["run", "-d", "--name", name, "--network", network, "--entrypoint", "/indexer"]
    for key, value in env.items():
        args += ["-e", f"{key}={value}"]
    _docker(*args, image)

    def _cleanup() -> None:
        subprocess.run(["docker", "rm", "-f", name], capture_output=True, timeout=60, check=False)
        httpx.delete(f"{ev.opensearch_url()}/{index}", timeout=30.0)

    world.add_cleanup(_cleanup)


@then("the new index holds that listing with its text and a 384 dimension embedding")
def replayed(world: World) -> None:
    lid = s.listing(world, LISTING_L).id
    index = h.bag(world)["replay_index"]

    def _doc():
        resp = httpx.get(f"{ev.opensearch_url()}/{index}/_doc/{lid}", timeout=15.0)
        if resp.status_code != 200 or not resp.json().get("found"):
            return None
        src = resp.json()["_source"]
        return src if len(src.get("embedding") or []) == ms.EMBED_DIM else None

    doc = s.eventually(_doc, f"listing {lid} replayed with an embedding into {index}", 300.0, 3.0)
    assert doc["title"] == s.listing(world, LISTING_L).title
    assert not doc.get("vector_pending")


_ = (parsers, time)  # keep the imports pytest-bdd step modules conventionally carry
