"""Unit tests (no stack): the deterministic TEI fake behaves as its docstring promises.

The real server runs on an ephemeral port in a thread; requests go over real HTTP.
"""

from __future__ import annotations

import importlib.util
import math
import threading
import time
from pathlib import Path

import httpx
import pytest

_PATH = Path(__file__).resolve().parents[1] / "fakes" / "tei_fake" / "server.py"
_spec = importlib.util.spec_from_file_location("tei_fake_server", _PATH)
fake = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(fake)


@pytest.fixture
def server():
    srv, state = fake.make_server("127.0.0.1", 0)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{srv.server_address[1]}", state
    srv.shutdown()
    srv.server_close()


def cosine(a: list[float], b: list[float]) -> float:
    return sum(x * y for x, y in zip(a, b, strict=True))


def test_vectors_are_deterministic_unit_and_384_wide():
    first = fake.embed_text("red running sneakers")
    assert first == fake.embed_text("red running sneakers")
    assert len(first) == 384
    assert math.isclose(math.sqrt(sum(v * v for v in first)), 1.0, abs_tol=1e-3)
    assert any(v != 0.0 for v in fake.embed_text(""))  # never the zero vector


def test_overlapping_texts_are_closer_than_unrelated_ones():
    base = fake.embed_text("red running sneakers")
    near = fake.embed_text("blue running sneakers")
    far = fake.embed_text("kitchen blender")
    assert cosine(base, near) > 0.5
    assert cosine(base, near) > cosine(base, far) + 0.4


def test_synonyms_and_alias_suffix_share_a_concept():
    assert cosine(fake.embed_text("trainers"), fake.embed_text("sneakers")) == pytest.approx(
        1.0, abs=1e-3
    )
    assert cosine(fake.embed_text("abc7zalias"), fake.embed_text("abc7")) == pytest.approx(
        1.0, abs=1e-3
    )
    assert cosine(fake.embed_text("abc7zalias"), fake.embed_text("abc8")) < 0.2


def test_directives_never_change_a_vector():
    plain = fake.embed_text("hello world")
    assert fake.embed_text("hello world [[fake status=500 delay=9]]") == plain


def test_parse_directive_last_wins():
    assert fake.parse_directive("[[fake status=500]] x [[fake status=503 delay=7]]") == {
        "status": "503",
        "delay": "7",
    }


def test_embed_native_returns_bare_list_in_order(server):
    base, _ = server
    resp = httpx.post(f"{base}/embed", json={"inputs": ["apple", "banana"]})
    assert resp.status_code == 200
    body = resp.json()
    assert isinstance(body, list) and len(body) == 2 and len(body[0]) == 384
    assert body[1] == fake.embed_text("banana")


def test_embed_accepts_a_single_string_and_texts_key(server):
    base, _ = server
    assert len(httpx.post(f"{base}/embed", json={"inputs": "apple"}).json()) == 1
    assert len(httpx.post(f"{base}/embed", json={"texts": ["a", "b", "c"]}).json()) == 3


def test_v1_embeddings_openai_shape(server):
    base, _ = server
    body = httpx.post(f"{base}/v1/embeddings", json={"input": ["hello world"]}).json()
    assert body["object"] == "list"
    assert body["data"][0]["index"] == 0
    assert body["data"][0]["embedding"] == fake.embed_text("hello world")


def test_status_directive_fails_the_whole_batch(server):
    base, _ = server
    resp = httpx.post(f"{base}/embed", json={"inputs": ["fine", "bad [[fake status=500]]"]})
    assert resp.status_code == 500
    assert (
        httpx.post(f"{base}/v1/embeddings", json={"input": ["x [[fake status=503]]"]}).status_code
        == 503
    )


def test_delay_directive_slows_the_call(server):
    base, _ = server
    started = time.monotonic()
    resp = httpx.post(f"{base}/embed", json={"inputs": ["slow [[fake delay=300]]"]}, timeout=5)
    assert resp.status_code == 200
    assert time.monotonic() - started >= 0.3


def test_rerank_is_a_sorted_bare_list_by_token_overlap(server):
    base, _ = server
    texts = ["blue jacket", "red sneakers", "red running sneakers"]
    body = httpx.post(f"{base}/rerank", json={"query": "red sneakers", "texts": texts}).json()
    assert isinstance(body, list)
    assert [r["index"] for r in body][:2] == [1, 2]  # both full overlaps keep input order
    assert body[-1] == {"index": 0, "score": 0.0}
    assert all(set(r) == {"index", "score"} for r in body)


def test_rerank_reverse_and_failure_directives(server):
    base, _ = server
    texts = ["a", "b", "c"]
    reverse = httpx.post(
        f"{base}/rerank", json={"query": "q [[fake rerank=reverse]]", "texts": texts}
    )
    assert [r["index"] for r in reverse.json()] == [2, 1, 0]
    failed = httpx.post(
        f"{base}/rerank", json={"query": "q [[fake rerank_status=500]]", "texts": texts}
    )
    assert failed.status_code == 500
    # a rerank-only directive leaves embed alone
    assert (
        httpx.post(f"{base}/embed", json={"inputs": ["q [[fake rerank_status=500]]"]}).status_code
        == 200
    )


def test_chat_and_generate_stand_in_for_vllm(server):
    base, _ = server
    chat = httpx.post(
        f"{base}/v1/chat/completions",
        json={"model": "m", "messages": [{"role": "user", "content": "hi"}]},
    ).json()
    assert chat["choices"][0]["message"]["content"].startswith("[tei-fake-vllm]")
    assert (
        httpx.post(f"{base}/generate", json={"prompt": "hi"})
        .json()["text"][0]
        .startswith("[tei-fake-vllm]")
    )


def test_request_log_filters_by_text_and_kind_and_resets(server):
    base, _ = server
    httpx.post(f"{base}/embed", json={"inputs": ["needle-one"]})
    httpx.post(f"{base}/embed", json={"inputs": ["haystack"]})
    httpx.post(f"{base}/rerank", json={"query": "needle-one", "texts": ["x"]})
    assert httpx.get(f"{base}/_requests", params={"contains": "needle-one"}).json()["count"] == 2
    only = httpx.get(f"{base}/_requests", params={"contains": "needle-one", "kind": "embed"}).json()
    assert only["count"] == 1 and only["requests"][0]["status"] == 200
    httpx.post(f"{base}/_reset")
    assert httpx.get(f"{base}/_requests").json()["count"] == 0


def test_global_mode_fails_every_call_until_cleared(server):
    base, _ = server
    httpx.post(f"{base}/_mode", json={"status": 503})
    assert httpx.post(f"{base}/embed", json={"inputs": ["x"]}).status_code == 503
    assert httpx.get(f"{base}/healthz").status_code == 200
    httpx.post(f"{base}/_mode", json={})
    assert httpx.post(f"{base}/embed", json={"inputs": ["x"]}).status_code == 200


def test_bad_json_is_a_400_and_unknown_path_a_404(server):
    base, _ = server
    assert (
        httpx.post(
            f"{base}/embed", content=b"{nope", headers={"Content-Type": "application/json"}
        ).status_code
        == 400
    )
    assert httpx.post(f"{base}/nope", json={}).status_code == 404
