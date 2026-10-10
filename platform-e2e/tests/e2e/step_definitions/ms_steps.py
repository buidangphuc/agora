"""Steps for modelserve/model_serving.feature (change add-platform-modelserve)."""

from __future__ import annotations

import time
from concurrent.futures import ThreadPoolExecutor

import httpx
import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import ms_support as ms
from tests.e2e.support.world import World


@pytest.fixture
def mctx() -> dict:
    return {}


# ── embeddings ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a client posts "{path}" to the router with two unique texts under the key "texts"'
    )
)
def post_two_texts(mctx: dict, path: str) -> None:
    mctx["texts"] = [f"apple {ms.uid()}", f"banana {ms.uid()}"]
    mctx["resp"] = ms.router_post(path, {"texts": mctx["texts"]})


@then(
    "the router answers 200 with embeddings that pass the team-ai vector validation at dimension 384"
)
def embeddings_conform(mctx: dict) -> None:
    resp = mctx["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    ms.conforms_to_team_ai_parser(resp.json(), count=len(mctx["texts"]))


@when(
    parsers.parse(
        'a client posts "{path}" to the router with the input "hello world" and a unique word'
    )
)
def post_openai(mctx: dict, path: str) -> None:
    mctx["texts"] = [f"hello world {ms.uid()}"]
    mctx["resp"] = ms.router_post(path, {"input": mctx["texts"]})


@then("the router answers 200 with an OpenAI list holding one 384 dimension embedding")
def openai_shape(mctx: dict) -> None:
    resp = mctx["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    body = resp.json()
    assert body.get("object") == "list", body.keys()
    assert len(body["data"]) == 1
    vector = body["data"][0]["embedding"]
    assert len(vector) == ms.EMBED_DIM and all(isinstance(x, (int, float)) for x in vector)


# ── cache ────────────────────────────────────────────────────────────────
@given("the router has embedded a unique text once and the upstream saw exactly one call for it")
def embed_once(mctx: dict) -> None:
    mctx["text"] = f"cached {ms.uid()}"
    first = ms.router_post("/embed", {"texts": [mctx["text"]]})
    assert first.status_code == 200, first.text[:300]
    mctx["vector"] = first.json()["embeddings"][0]
    assert len(ms.fake_calls(mctx["text"], "embed")) == 1  # positive control: a miss reaches TEI


@when('a client posts the same text to "/embed" again')
def embed_again(mctx: dict) -> None:
    mctx["resp"] = ms.router_post("/embed", {"texts": [mctx["text"]]})


@then("the router returns the same vector and the upstream saw no second call for that text")
def cache_hit(mctx: dict) -> None:
    resp = mctx["resp"]
    assert resp.status_code == 200, resp.text[:300]
    assert resp.json()["embeddings"][0] == mctx["vector"]
    calls = ms.fake_calls(mctx["text"], "embed")
    assert len(calls) == 1, f"the upstream was called {len(calls)} times for a cached text"


@given("the router has cached the vectors of two unique texts")
def cache_two(mctx: dict) -> None:
    mctx["c1"], mctx["c2"] = f"warm one {ms.uid()}", f"warm two {ms.uid()}"
    resp = ms.router_post("/embed", {"texts": [mctx["c1"], mctx["c2"]]})
    assert resp.status_code == 200, resp.text[:300]
    assert len(ms.fake_calls(mctx["c1"], "embed")) == 1  # positive control


@when("a client posts those two texts and two new ones in an interleaved order")
def post_mixed(mctx: dict) -> None:
    mctx["m1"], mctx["m2"] = f"fresh one {ms.uid()}", f"fresh two {ms.uid()}"
    mctx["order"] = [mctx["m1"], mctx["c1"], mctx["m2"], mctx["c2"]]
    mctx["resp"] = ms.router_post("/embed", {"texts": mctx["order"]})


@then("the upstream received only the two new texts in their request order")
def only_misses(mctx: dict) -> None:
    assert mctx["resp"].status_code == 200, mctx["resp"].text[:300]
    calls = ms.fake_calls(mctx["m1"], "embed")
    assert len(calls) == 1, f"expected one upstream call carrying the misses, got {len(calls)}"
    assert ms.fake_inputs(calls[0]) == [mctx["m1"], mctx["m2"]]


@then("the router returned four vectors in the request order")
def four_in_order(mctx: dict) -> None:
    got = mctx["resp"].json()["embeddings"]
    assert got == ms.fake_vectors(mctx["order"])


@then("posting the same four texts again reaches the upstream not at all")
def now_all_cached(mctx: dict) -> None:
    before = len(ms.fake_calls(mctx["m1"], "embed"))
    again = ms.router_post("/embed", {"texts": mctx["order"]})
    assert again.status_code == 200 and again.json() == mctx["resp"].json()
    # the ground-truth call above (fake_vectors) also carries m1, so compare with that in mind
    after = [
        c
        for c in ms.fake_calls(mctx["m1"], "embed")
        if ms.fake_inputs(c) == [mctx["m1"], mctx["m2"]]
    ]
    assert len(after) == 1 and before >= 1, "the cached texts went upstream again"


# ── proxying ─────────────────────────────────────────────────────────────
@when(
    'a client posts a rerank request for "shoes" with the texts "red sneakers" and "blue jacket" '
    "to the router"
)
def post_rerank(mctx: dict) -> None:
    mctx["rerank"] = {"query": f"shoes {ms.uid()}", "texts": ["red sneakers", "blue jacket"]}
    mctx["resp"] = ms.router_post("/rerank", mctx["rerank"])


@then("the router returns the ranked results of the upstream with the sneakers first")
def rerank_results(mctx: dict) -> None:
    resp = mctx["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    direct = ms.fake_post("/rerank", mctx["rerank"]).json()
    assert resp.json() == direct
    assert [r["index"] for r in direct] == [0, 1], direct


@then("the upstream received the rerank payload unchanged")
def rerank_payload(mctx: dict) -> None:
    import json

    calls = ms.fake_calls(mctx["rerank"]["query"], "rerank")
    assert calls, "the upstream never saw the rerank request"
    forwarded = json.loads(calls[0]["body"])
    assert forwarded == mctx["rerank"], forwarded


@when("a client posts an OpenAI chat completion with a unique message to the router")
def post_chat(mctx: dict) -> None:
    mctx["chat"] = {
        "model": "stand-in",
        "messages": [{"role": "user", "content": f"say hi {ms.uid()}"}],
    }
    mctx["resp"] = ms.router_post("/v1/chat/completions", mctx["chat"])


@then("the router returns the completion of the vLLM upstream stand-in")
def chat_reply(mctx: dict) -> None:
    resp = mctx["resp"]
    assert resp.status_code == 200, f"{resp.status_code}: {resp.text[:300]}"
    content = resp.json()["choices"][0]["message"]["content"]
    assert content.startswith("[tei-fake-vllm]"), content


@then("the upstream received the chat payload unchanged")
def chat_payload(mctx: dict) -> None:
    import json

    word = mctx["chat"]["messages"][0]["content"].split()[-1]
    calls = ms.fake_calls(word, "chat")
    assert calls and calls[0]["path"] == "/v1/chat/completions"
    assert json.loads(calls[0]["body"]) == mctx["chat"]


# ── backpressure ─────────────────────────────────────────────────────────
@when("40 slow embedding requests arrive at the router at the same moment")
def flood(mctx: dict) -> None:
    # A router restarted by an earlier destructive scenario answers /healthz before its published
    # port takes a burst of new connections: settle on a few sequential embeds first, so the flood
    # measures admission control rather than the restart.
    streak, deadline = 0, time.monotonic() + 60
    while streak < 5 and time.monotonic() < deadline:
        try:
            ok = ms.router_post("/embed", {"texts": [f"warm {ms.uid()}"]}, timeout=10.0).is_success
        except httpx.HTTPError:
            ok = False
        streak = streak + 1 if ok else 0
        time.sleep(0.2)

    def one(i: int) -> tuple[int, str | None]:
        text = f"slow {i} {ms.uid()} [[fake delay=2500]]"
        try:
            r = httpx.post(ms.router_url() + "/embed", json={"texts": [text]}, timeout=60.0)
        except httpx.HTTPError as exc:  # pragma: no cover - surfaced by the assertions below
            return -1, f"{type(exc).__name__}: {exc}"
        return r.status_code, r.headers.get("retry-after")

    with ThreadPoolExecutor(max_workers=40) as pool:
        mctx["results"] = list(pool.map(one, range(40)))


@then("some of them are answered 429 with a Retry-After header")
def some_429(mctx: dict) -> None:
    rejected = [r for r in mctx["results"] if r[0] == 429]
    assert (
        rejected
    ), f"no 429 among {mctx['results'][:3]} ... {sorted(r[0] for r in mctx['results'])}"
    assert all(r[1] for r in rejected), "a 429 without Retry-After"


@then("the admitted ones still complete with 200")
def admitted_ok(mctx: dict) -> None:
    codes = {r[0] for r in mctx["results"]}
    assert 200 in codes, f"nothing was admitted: {sorted(codes)}"
    assert codes <= {200, 429}, f"unexpected statuses: {sorted(codes)}"


@then("the router serves a normal request once the load has drained")
def drained(mctx: dict, world: World) -> None:
    deadline = time.monotonic() + 30
    last = None
    while time.monotonic() < deadline:
        last = ms.router_post("/embed", {"texts": [f"after the flood {ms.uid()}"]})
        if last.status_code == 200:
            return
        time.sleep(1)
    raise AssertionError(f"the router did not recover: {last.status_code} {last.text[:200]}")
