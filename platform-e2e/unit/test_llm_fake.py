"""Unit tests (no stack): the fault-injecting LLM fake behaves as design D8 promises.

The real server runs on an ephemeral port in a thread; requests go over real HTTP so the
chunked-stream and dropped-connection behaviour is exercised for real.
"""

from __future__ import annotations

import importlib.util
import json
import threading
import time
from pathlib import Path

import httpx
import pytest

_PATH = Path(__file__).resolve().parents[1] / "fakes" / "llm_fake" / "server.py"
_spec = importlib.util.spec_from_file_location("llm_fake_server", _PATH)
fake = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(fake)


@pytest.fixture
def server():
    srv, state = fake.make_server("127.0.0.1", 0)
    thread = threading.Thread(target=srv.serve_forever, daemon=True)
    thread.start()
    base = f"http://127.0.0.1:{srv.server_address[1]}"
    yield base, state
    state.stop.set()
    srv.shutdown()
    srv.server_close()


def chat(base, model="primary", text="hello", stream=True, usage=True, timeout=10):
    body = {"model": model, "messages": [{"role": "user", "content": text}], "stream": stream}
    if stream and usage:
        body["stream_options"] = {"include_usage": True}
    return httpx.post(f"{base}/v1/chat/completions", json=body, timeout=timeout)


def sse_events(resp):
    out = []
    for line in resp.text.splitlines():
        if line.startswith("data: "):
            out.append(line[6:])
    return out


def content(resp):
    parts = []
    for ev in sse_events(resp):
        if ev == "[DONE]":
            continue
        for choice in json.loads(ev)["choices"]:
            parts.append(choice["delta"].get("content", ""))
    return "".join(parts)


# ── pure helpers ─────────────────────────────────────────────────────────
def test_directive_parsing_and_precedence():
    text = "hi [[fake primary=429 fb1=ok fb2=hang]]"
    assert fake.parse_directive(text) == {"primary": "429", "fb1": "ok", "fb2": "hang"}
    msgs = [{"role": "user", "content": text}]
    assert fake.resolve_mode("primary", msgs, {"primary": "500"}) == "429"  # directive wins
    assert fake.resolve_mode("fb1", msgs, {"fb1": "500"}) == "ok"
    assert fake.resolve_mode("other", msgs, {"other": "500"}) == "500"  # global mode
    assert fake.resolve_mode("other", msgs, {}) == "ok"  # default
    assert (
        fake.resolve_mode("primary", [{"role": "user", "content": "[[fake primary=zzz]]"}], {})
        == "ok"
    )


def test_directive_is_read_from_the_last_user_message_only():
    msgs = [
        {"role": "user", "content": "[[fake primary=500]]"},
        {"role": "assistant", "content": "[primary] ok"},
        {"role": "user", "content": "second"},
    ]
    assert fake.resolve_mode("primary", msgs, {}) == "ok"


# ── streaming ────────────────────────────────────────────────────────────
def test_streams_a_prefixed_reply_with_usage_in_the_final_chunk(server):
    base, _ = server
    resp = chat(base, "fb1")
    assert resp.status_code == 200
    assert content(resp).startswith("[fb1] ")
    events = sse_events(resp)
    assert events[-1] == "[DONE]"
    last = json.loads(events[-2])
    assert last["choices"] == []
    assert last["usage"] == {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18}


def test_no_usage_chunk_without_include_usage(server):
    base, _ = server
    resp = chat(base, usage=False)
    assert "usage" not in resp.text


def test_non_streaming_reply_has_usage(server):
    base, _ = server
    body = chat(base, "fb2", stream=False).json()
    assert body["choices"][0]["message"]["content"].startswith("[fb2] ")
    assert body["usage"]["prompt_tokens"] == 11 and body["usage"]["completion_tokens"] == 7


@pytest.mark.parametrize("mode,status", [("429", 429), ("500", 500), ("400", 400)])
def test_error_modes_answer_the_status_with_provider_text(server, mode, status):
    base, _ = server
    resp = chat(base, text=f"x [[fake primary={mode}]]")
    assert resp.status_code == status
    assert fake.PROVIDER_ERROR_TEXT in resp.text


def test_break2_streams_two_chunks_then_drops_the_connection(server):
    base, _ = server
    got = []
    with pytest.raises(httpx.HTTPError):
        with httpx.stream(
            "POST",
            f"{base}/v1/chat/completions",
            json={
                "model": "primary",
                "stream": True,
                "messages": [{"role": "user", "content": "x [[fake primary=break2]]"}],
            },
            timeout=10,
        ) as resp:
            for line in resp.iter_lines():
                if line.startswith("data: "):
                    got.append(line[6:])
    deltas = [
        json.loads(e)["choices"][0]["delta"].get("content")
        for e in got
        if e != "[DONE]" and json.loads(e)["choices"][0]["delta"].get("content")
    ]
    assert len(deltas) == 2 and deltas[0] == "[primary] "
    assert "[DONE]" not in got


def test_hang_never_answers_and_notes_the_disconnect(server):
    base, state = server
    with pytest.raises(httpx.ReadTimeout):
        chat(base, text="x [[fake primary=hang]]", timeout=0.5)
    deadline = time.monotonic() + 3
    while time.monotonic() < deadline:
        if state.requests and state.requests[0]["client_disconnected"]:
            break
        time.sleep(0.05)
    assert state.requests[0]["mode"] == "hang"
    assert state.requests[0]["client_disconnected"] is True


# ── control endpoints ────────────────────────────────────────────────────
def test_global_mode_set_cleared_and_reset(server):
    base, _ = server
    assert httpx.post(f"{base}/_mode", json={"primary": "500", "fb1": "429"}).json() == {
        "primary": "500",
        "fb1": "429",
    }
    assert chat(base, "primary").status_code == 500
    assert chat(base, "fb1").status_code == 429
    assert chat(base, "fb2").status_code == 200
    assert httpx.post(f"{base}/_mode", json={"primary": "ok"}).json() == {"fb1": "429"}
    assert chat(base, "primary").status_code == 200
    assert httpx.post(f"{base}/_reset").status_code == 200
    assert httpx.get(f"{base}/_mode").json() == {}
    assert httpx.get(f"{base}/_requests").json()["requests"] == []


def test_unknown_mode_is_rejected(server):
    base, _ = server
    resp = httpx.post(f"{base}/_mode", json={"primary": "explode"})
    assert resp.status_code == 400


def test_requests_endpoint_filters_on_the_body(server):
    base, _ = server
    chat(base, "primary", text="needle-aaa")
    chat(base, "fb1", text="other-bbb")
    rows = httpx.get(f"{base}/_requests", params={"contains": "needle-aaa"}).json()["requests"]
    assert [r["model"] for r in rows] == ["primary"]
    assert rows[0]["body"]["messages"][-1]["content"] == "needle-aaa"
    assert len(httpx.get(f"{base}/_requests").json()["requests"]) == 2


def test_langfuse_ingestion_and_otlp_are_recorded(server):
    base, _ = server
    r = httpx.post(f"{base}/api/public/ingestion", json={"batch": [{"id": "e2e-trace-zzz"}]})
    assert r.status_code == 207 and r.json() == {"successes": [], "errors": []}
    assert (
        httpx.post(f"{base}/api/public/otel/v1/traces", content=b"\x0a\x03e2e-otlp").status_code
        == 200
    )
    hits = httpx.get(f"{base}/_ingested", params={"contains": "e2e-trace-zzz"}).json()["ingested"]
    assert [h["path"] for h in hits] == ["/api/public/ingestion"]
    assert (
        len(httpx.get(f"{base}/_ingested", params={"contains": "e2e-otlp"}).json()["ingested"]) == 1
    )
    assert len(httpx.get(f"{base}/_ingested").json()["ingested"]) == 2
    assert httpx.get(f"{base}/api/public/v2/prompts/x").status_code == 404
