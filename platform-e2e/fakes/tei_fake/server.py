"""Deterministic, fault-injecting fake of Hugging Face TEI (embed + rerank) for the e2e stack.

Stand-in for the real TEI image, which the e2e stack does not download. Stdlib only
(``http.server`` + threads). Mirrors ``platform-e2e/fakes/llm_fake``.

Routes
------
POST /embed                 TEI native: ``{"inputs": "t" | ["t", ...]}`` (``texts`` also accepted)
                            -> a bare list of vectors
POST /v1/embeddings         OpenAI shape: ``{"input": ...}`` -> ``{"object": "list", "data": [...]}``
POST /rerank                TEI native: ``{"query", "texts"}`` -> bare list ``[{"index", "score"}]``,
                            best first
POST /v1/chat/completions   stand-in for the vLLM upstream of the router (canned, non streaming);
POST /generate              the reply starts with ``[tei-fake-vllm]``
GET  /_requests?contains=<s>&kind=<k>   recorded requests whose raw body contains ``s``
POST /_mode                 global fault, e.g. ``{"status": 503, "delay_ms": 0}`` (``{}`` clears)
GET  /_mode                 the current global fault
POST /_reset                clear the request log and the global fault
GET  /healthz               liveness

Vectors (dim 384)
-----------------
A text becomes a bag of concepts, L2-normalised: every word maps (sha256) to a dense
pseudo-random 384 dimension direction. Words of one *concept* map alike, so texts that share
words or synonyms have a high cosine and unrelated texts are near orthogonal (|cos| < ~0.1). Concepts:

* ``CONCEPTS`` below (sneakers = trainers = shoes = kicks, laptop = notebook, ...);
* any word ending in ``zalias`` is the concept of what precedes it (``abc7zalias`` ~ ``abc7``),
  so a test can build a per-run unique pair that is lexically different yet semantically equal.

Rerank score = share of the query's words found in the text; ties keep input order.

Directives (in a text of an embed call, in the query of a rerank call, in a chat message;
removed before hashing/scoring, so they never change a vector)
-----------------------------------------------------------------------------------------
``[[fake status=500]]``         the call answers that HTTP status (embed/rerank/chat)
``[[fake delay=3000]]``         the call answers after that many milliseconds
``[[fake rerank_status=500]]``  like ``status`` but only for /rerank
``[[fake rerank_delay=3000]]``  like ``delay`` but only for /rerank
``[[fake rerank=reverse]]``     /rerank scores the LAST text highest (reverses the input order)
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import re
import threading
import time
from functools import lru_cache
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

DIM = 384
MODEL = "tei-fake"
CONCEPTS = (
    ("sneakers", "trainers", "shoes", "kicks"),
    ("laptop", "notebook"),
    ("phone", "smartphone", "mobile"),
    ("jacket", "coat"),
)
ALIAS_SUFFIX = "zalias"
CHAT_REPLY = "[tei-fake-vllm] xin chào, đây là câu trả lời thử nghiệm"

_CONCEPT_OF = {word: group[0] for group in CONCEPTS for word in group}
_DIRECTIVE = re.compile(r"\[\[fake\s+([^\]]*)\]\]")
_WORD = re.compile(r"\w+", re.UNICODE)


# ── pure functions ───────────────────────────────────────────────────────
def parse_directive(text: str) -> dict[str, str]:
    """``[[fake status=500 delay=5]]`` -> ``{"status": "500", "delay": "5"}`` (last one wins)."""
    found: dict[str, str] = {}
    for block in _DIRECTIVE.findall(text or ""):
        for pair in block.split():
            key, sep, value = pair.partition("=")
            if sep and key:
                found[key] = value
    return found


def strip_directives(text: str) -> str:
    return _DIRECTIVE.sub(" ", text or "")


def tokens(text: str) -> list[str]:
    return _WORD.findall(strip_directives(text).lower())


def concept(word: str) -> str:
    if word.endswith(ALIAS_SUFFIX) and len(word) > len(ALIAS_SUFFIX):
        return word[: -len(ALIAS_SUFFIX)]
    return _CONCEPT_OF.get(word, word)


@lru_cache(maxsize=4096)
def concept_vector(name: str) -> tuple[float, ...]:
    """A deterministic pseudo-random DENSE direction in [-1, 1]^DIM for one concept.

    Dense on purpose: a one-hot (hashed bag) vector is orthogonal to almost every other one, so
    a k-NN index full of them is a plateau of equal scores and its HNSW graph cannot be
    navigated (measured recall of a planted neighbour: ~30%, never above 70% even at k=50). Random
    dense directions keep unrelated texts at cosine ~0 (+-0.05 in 384 dims) and equal concepts at
    exactly the same vector, while giving the graph real gradients (recall ~100%).
    """
    out: list[float] = []
    block = 0
    while len(out) < DIM:
        digest = hashlib.sha256(f"{name}\x00{block}".encode()).digest()
        out.extend(int.from_bytes(digest[i : i + 4], "big") / 2**31 - 1.0 for i in range(0, 32, 4))
        block += 1
    return tuple(out[:DIM])


def embed_text(text: str) -> list[float]:
    """The deterministic unit vector of ``text`` (never the zero vector)."""
    vec = [0.0] * DIM
    words = tokens(text) or ["\x00blank"]
    for word in words:
        for i, value in enumerate(concept_vector(concept(word))):
            vec[i] += value
    norm = math.sqrt(sum(v * v for v in vec))
    return [round(v / norm, 6) for v in vec]


def rerank_scores(query: str, texts: list[str], mode: str = "") -> list[dict]:
    """TEI ``/rerank`` result: ``[{"index", "score"}]`` sorted best first."""
    if mode == "reverse":
        scored = [(i, (i + 1) / max(len(texts), 1)) for i in range(len(texts))]
    else:
        wanted = {concept(w) for w in tokens(query)}
        scored = []
        for i, text in enumerate(texts):
            have = {concept(w) for w in tokens(text)}
            scored.append((i, len(wanted & have) / len(wanted) if wanted else 0.0))
    scored.sort(key=lambda pair: (-pair[1], pair[0]))
    return [{"index": i, "score": round(score, 6)} for i, score in scored]


def _as_list(value: object) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [v if isinstance(v, str) else str(v) for v in value]
    return []


# ── state ────────────────────────────────────────────────────────────────
class State:
    """Everything the fake remembers; one instance per server."""

    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.requests: list[dict] = []
        self.fault: dict = {}
        self.seq = 0

    def reset(self) -> None:
        with self.lock:
            self.requests.clear()
            self.fault = {}

    def record(self, kind: str, path: str, body: bytes, status: int) -> None:
        with self.lock:
            self.seq += 1
            self.requests.append(
                {
                    "seq": self.seq,
                    "kind": kind,
                    "path": path,
                    "status": status,
                    "at": time.time(),
                    "body": body.decode("utf-8", errors="replace"),
                }
            )


def _int(value: object, default: int = 0) -> int:
    try:
        return int(str(value))
    except ValueError:
        return default


def resolve_fault(directives: list[dict[str, str]], fault: dict, rerank: bool) -> tuple[int, int]:
    """(HTTP status, delay in ms) from directives, then the global fault. 200 / 0 = healthy."""
    status, delay = 200, 0
    for found in directives:
        status = max(status, _int(found.get("status"), 200))
        delay = max(delay, _int(found.get("delay")))
        if rerank:
            status = max(status, _int(found.get("rerank_status"), 200))
            delay = max(delay, _int(found.get("rerank_delay")))
    if status == 200 and fault.get("status"):
        status = _int(fault["status"], 200)
    delay = max(delay, _int(fault.get("delay_ms")))
    return status, delay


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "tei-fake/1"
    state: State  # set on the generated subclass

    def log_message(self, fmt: str, *args) -> None:  # noqa: A003 - quiet; /_requests is the log
        return

    def _body(self) -> bytes:
        length = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(length) if length else b""

    def _json(self, status: int, payload: object) -> None:
        data = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def _path(self) -> str:
        return urlparse(self.path).path

    # ── GET ──────────────────────────────────────────────────────────────
    def do_GET(self) -> None:  # noqa: N802
        path = self._path()
        query = parse_qs(urlparse(self.path).query)
        if path == "/healthz":
            self._json(200, {"status": "ok"})
        elif path == "/_mode":
            self._json(200, self.state.fault)
        elif path == "/_requests":
            needle = (query.get("contains") or [""])[0]
            kind = (query.get("kind") or [""])[0]
            with self.state.lock:
                found = [
                    r
                    for r in self.state.requests
                    if needle in r["body"] and (not kind or r["kind"] == kind)
                ]
            self._json(200, {"requests": found, "count": len(found)})
        else:
            self._json(404, {"error": "not found"})

    # ── POST ─────────────────────────────────────────────────────────────
    def do_POST(self) -> None:  # noqa: N802
        path = self._path()
        raw = self._body()
        if path == "/_reset":
            self.state.reset()
            self._json(200, {"status": "reset"})
            return
        if path == "/_mode":
            try:
                body = json.loads(raw or b"{}")
            except ValueError:
                self._json(400, {"error": "invalid json"})
                return
            with self.state.lock:
                self.state.fault = body if isinstance(body, dict) else {}
            self._json(200, self.state.fault)
            return
        try:
            body = json.loads(raw or b"{}")
        except ValueError:
            self._json(400, {"error": "invalid json"})
            return
        if not isinstance(body, dict):
            self._json(400, {"error": "json object expected"})
            return
        if path in ("/embed", "/v1/embeddings"):
            self._embed(path, raw, body)
        elif path == "/rerank":
            self._rerank(path, raw, body)
        elif path in ("/v1/chat/completions", "/generate"):
            self._chat(path, raw, body)
        else:
            self._json(404, {"error": "not found"})

    def _gate(self, kind: str, path: str, raw: bytes, texts: list[str], rerank: bool) -> bool:
        """Apply directives and the global fault. True = the caller answers normally."""
        with self.state.lock:
            fault = dict(self.state.fault)
        status, delay = resolve_fault([parse_directive(t) for t in texts], fault, rerank)
        if delay:
            time.sleep(delay / 1000.0)
        self.state.record(kind, path, raw, status)
        if status != 200:
            self._json(status, {"error": "fake-provider-injected", "status": status})
            return False
        return True

    def _embed(self, path: str, raw: bytes, body: dict) -> None:
        texts = _as_list(body.get("inputs", body.get("input", body.get("texts"))))
        if not self._gate("embed", path, raw, texts, rerank=False):
            return
        vectors = [embed_text(t) for t in texts]
        if path == "/v1/embeddings":
            self._json(
                200,
                {
                    "object": "list",
                    "model": MODEL,
                    "data": [
                        {"object": "embedding", "index": i, "embedding": v}
                        for i, v in enumerate(vectors)
                    ],
                },
            )
        else:
            self._json(200, vectors)

    def _rerank(self, path: str, raw: bytes, body: dict) -> None:
        query = str(body.get("query") or "")
        texts = _as_list(body.get("texts"))
        if not self._gate("rerank", path, raw, [query], rerank=True):
            return
        mode = parse_directive(query).get("rerank", "")
        self._json(200, rerank_scores(query, texts, mode))

    def _chat(self, path: str, raw: bytes, body: dict) -> None:
        texts = [
            m.get("content", "")
            for m in body.get("messages") or []
            if isinstance(m, dict) and isinstance(m.get("content"), str)
        ]
        texts.append(str(body.get("prompt") or body.get("inputs") or ""))
        if not self._gate("chat", path, raw, texts, rerank=False):
            return
        if path == "/generate":
            self._json(200, {"text": [CHAT_REPLY], "model": MODEL})
            return
        self._json(
            200,
            {
                "id": "chatcmpl-tei-fake",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": body.get("model") or MODEL,
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": CHAT_REPLY},
                        "finish_reason": "stop",
                    }
                ],
                "usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18},
            },
        )


def make_server(host: str = "127.0.0.1", port: int = 0) -> tuple[ThreadingHTTPServer, State]:
    state = State()
    handler = type("BoundHandler", (Handler,), {"state": state})
    server = ThreadingHTTPServer((host, port), handler)
    server.daemon_threads = True
    return server, state


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--host", default="0.0.0.0")  # noqa: S104 - container-local fake
    parser.add_argument("--port", type=int, default=8110)
    args = parser.parse_args(argv)
    server, _ = make_server(args.host, args.port)
    print(f"tei-fake listening on {args.host}:{server.server_address[1]}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
