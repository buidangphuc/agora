"""Fault-injecting, OpenAI-compatible fake LLM provider for the e2e stack.

Change ai-path-resilience, design D8. Stdlib only (``http.server`` + threads), so the image
is a bare ``python:3.12-slim`` with this one file.

Routes
------
POST /v1/chat/completions         streaming (SSE) and non-streaming chat completions
GET  /v1/models                   a minimal model list (some SDKs probe it)
GET  /_requests?contains=<s>      recorded chat requests whose raw body contains ``s``
POST /_mode                       set global per-target modes, e.g. {"primary": "500"}
GET  /_mode                       the current global modes
POST /_reset                      clear recorded requests, ingested bodies and global modes
POST /api/public/ingestion        Langfuse batch ingestion (recorded, answered 207)
POST /api/public/otel/v1/traces   OTLP/HTTP trace export (recorded, answered 200)
GET  /_ingested?contains=<s>      recorded ingestion / OTLP bodies containing ``s``
GET  /healthz                     liveness

Behaviour
---------
The target is the request's ``model`` field. Its mode comes from, in order: a directive in
the last user message (``[[fake primary=429 fb1=ok fb2=ok]]``, keys are model names), then
the global mode set with ``POST /_mode``, then ``ok``.

Modes: ``ok`` | ``429`` | ``500`` | ``400`` | ``hang`` | ``break2``. ``break2`` streams two
content chunks and then drops the connection mid-body (no terminating chunk, no ``[DONE]``).
``hang`` accepts the request and never answers until the client goes away.

Replies are ``"[<model>] "`` followed by words, so a test can tell which target answered.
Usage is 11 prompt / 7 completion tokens, in the final chunk when the request set
``stream_options.include_usage`` and always in a non-streaming reply.
"""

from __future__ import annotations

import argparse
import gzip
import json
import re
import select
import socket
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

MODES = ("ok", "429", "500", "400", "hang", "break2")
PROMPT_TOKENS = 11
COMPLETION_TOKENS = 7
REPLY_WORDS = ("xin", "chào", "đây", "là", "câu", "trả", "lời", "thử", "nghiệm")
# Text the fake puts in its error bodies; tests assert it never reaches a client.
PROVIDER_ERROR_TEXT = "fake-provider-secret-detail"
HANG_LIMIT_S = 120.0

_DIRECTIVE = re.compile(r"\[\[fake\s+([^\]]*)\]\]")


class State:
    """Everything the fake remembers; one instance per server."""

    def __init__(self) -> None:
        self.lock = threading.Lock()
        self.requests: list[dict] = []
        self.ingested: list[dict] = []
        self.modes: dict[str, str] = {}
        self.seq = 0
        self.stop = threading.Event()

    def reset(self) -> None:
        with self.lock:
            self.requests.clear()
            self.ingested.clear()
            self.modes.clear()

    def next_seq(self) -> int:
        with self.lock:
            self.seq += 1
            return self.seq


def parse_directive(text: str) -> dict[str, str]:
    """``[[fake primary=429 fb1=ok]]`` -> ``{"primary": "429", "fb1": "ok"}`` (last one wins)."""
    found: dict[str, str] = {}
    for block in _DIRECTIVE.findall(text or ""):
        for pair in block.split():
            key, sep, value = pair.partition("=")
            if sep and key:
                found[key] = value
    return found


def last_user_text(messages: list) -> str:
    for message in reversed(messages or []):
        if isinstance(message, dict) and message.get("role") == "user":
            content = message.get("content")
            if isinstance(content, str):
                return content
            if isinstance(content, list):  # content parts
                return " ".join(
                    p.get("text", "") for p in content if isinstance(p, dict) and p.get("text")
                )
    return ""


def resolve_mode(model: str, messages: list, global_modes: dict[str, str]) -> str:
    directive = parse_directive(last_user_text(messages))
    mode = directive.get(model) or global_modes.get(model) or "ok"
    return mode if mode in MODES else "ok"


def reply_words(model: str) -> list[str]:
    return [f"[{model}] ", *(w + " " for w in REPLY_WORDS)]


def usage() -> dict:
    return {
        "prompt_tokens": PROMPT_TOKENS,
        "completion_tokens": COMPLETION_TOKENS,
        "total_tokens": PROMPT_TOKENS + COMPLETION_TOKENS,
    }


def _chunk(cid: str, model: str, created: int, delta: dict, finish: str | None) -> dict:
    return {
        "id": cid,
        "object": "chat.completion.chunk",
        "created": created,
        "model": model,
        "choices": [{"index": 0, "delta": delta, "finish_reason": finish}],
    }


def _decode(raw: bytes, encoding: str | None) -> str:
    if encoding and "gzip" in encoding.lower():
        try:
            raw = gzip.decompress(raw)
        except OSError:
            pass
    return raw.decode("utf-8", errors="replace")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    server_version = "llm-fake/1"
    state: State  # set on the generated subclass

    def log_message(self, fmt: str, *args) -> None:  # noqa: A003 - quiet; /_requests is the log
        return

    # ── plumbing ─────────────────────────────────────────────────────────
    def _body(self) -> bytes:
        length = int(self.headers.get("Content-Length") or 0)
        return self.rfile.read(length) if length else b""

    def _json(self, status: int, payload: object, headers: dict[str, str] | None = None) -> None:
        data = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        for key, value in (headers or {}).items():
            self.send_header(key, value)
        self.end_headers()
        self.wfile.write(data)

    def _query(self) -> dict[str, list[str]]:
        return parse_qs(urlparse(self.path).query)

    def _path(self) -> str:
        return urlparse(self.path).path

    def _client_gone(self) -> bool:
        try:
            readable, _, _ = select.select([self.connection], [], [], 0)
            if not readable:
                return False
            return self.connection.recv(1, socket.MSG_PEEK) == b""
        except (OSError, ValueError):
            return True

    def _write_chunk(self, payload: bytes) -> None:
        self.wfile.write(f"{len(payload):X}\r\n".encode() + payload + b"\r\n")
        self.wfile.flush()

    def _sse(self, obj: object) -> None:
        data = obj if isinstance(obj, str) else json.dumps(obj, ensure_ascii=False)
        self._write_chunk(f"data: {data}\n\n".encode())

    # ── routes ───────────────────────────────────────────────────────────
    def do_GET(self) -> None:  # noqa: N802
        path = self._path()
        if path == "/healthz":
            self._json(200, {"status": "ok"})
        elif path == "/_requests":
            needle = (self._query().get("contains") or [""])[0]
            with self.state.lock:
                rows = [r for r in self.state.requests if needle in r["raw"]]
            self._json(
                200, {"requests": [{k: v for k, v in r.items() if k != "raw"} for r in rows]}
            )
        elif path == "/_ingested":
            needle = (self._query().get("contains") or [""])[0]
            with self.state.lock:
                rows = [r for r in self.state.ingested if needle in r["body"]]
            self._json(200, {"ingested": rows})
        elif path == "/_mode":
            with self.state.lock:
                self._json(200, dict(self.state.modes))
        elif path == "/v1/models":
            self._json(200, {"object": "list", "data": [{"id": "primary", "object": "model"}]})
        elif path == "/api/public/projects":  # Langfuse auth_check
            self._json(200, {"data": [{"id": "fake", "name": "fake"}]})
        elif path.startswith("/api/public/"):  # prompt fetch etc.: not found -> SDK falls back
            self._json(404, {"message": "not found"})
        else:
            self._json(404, {"error": {"message": "not found"}})

    def do_POST(self) -> None:  # noqa: N802
        path = self._path()
        raw = self._body()
        if path == "/v1/chat/completions":
            self._chat(raw)
        elif path == "/_mode":
            self._set_mode(raw)
        elif path == "/_reset":
            self.state.reset()
            self._json(200, {"ok": True})
        elif path == "/api/public/ingestion":
            self._record_ingest(path, raw)
            self._json(207, {"successes": [], "errors": []})
        elif path == "/api/public/otel/v1/traces":
            self._record_ingest(path, raw)
            self._json(200, {"partialSuccess": {}})
        else:
            self._json(404, {"error": {"message": "not found"}})

    def _set_mode(self, raw: bytes) -> None:
        try:
            body = json.loads(raw or b"{}")
        except ValueError:
            return self._json(400, {"error": "invalid json"})
        if not isinstance(body, dict):
            return self._json(400, {"error": "expected an object"})
        bad = {k: v for k, v in body.items() if v not in (None, "", *MODES)}
        if bad:
            return self._json(400, {"error": f"unknown modes {bad}", "modes": list(MODES)})
        with self.state.lock:
            for model, mode in body.items():
                if mode in (None, "", "ok"):
                    self.state.modes.pop(model, None)
                else:
                    self.state.modes[model] = mode
            self._json(200, dict(self.state.modes))

    def _record_ingest(self, path: str, raw: bytes) -> None:
        text = _decode(raw, self.headers.get("Content-Encoding"))
        seq = self.state.next_seq()
        with self.state.lock:
            self.state.ingested.append({"seq": seq, "path": path, "body": text})

    # ── chat completions ─────────────────────────────────────────────────
    def _chat(self, raw: bytes) -> None:
        text = raw.decode("utf-8", errors="replace")
        try:
            body = json.loads(text)
        except ValueError:
            return self._json(400, {"error": {"message": "invalid json"}})
        model = str(body.get("model", ""))
        messages = body.get("messages") or []
        with self.state.lock:
            global_modes = dict(self.state.modes)
        mode = resolve_mode(model, messages, global_modes)
        record = {
            "seq": self.state.next_seq(),
            "time": time.time(),
            "model": model,
            "mode": mode,
            "stream": bool(body.get("stream")),
            "body": body,
            "raw": text,
            "client_disconnected": False,
        }
        with self.state.lock:
            self.state.requests.append(record)

        if mode == "429":
            return self._json(
                429,
                {
                    "error": {
                        "message": f"{PROVIDER_ERROR_TEXT}: rate limit reached",
                        "type": "rate_limit_exceeded",
                        "code": "rate_limit_exceeded",
                    }
                },
            )
        if mode == "500":
            return self._json(
                500,
                {"error": {"message": f"{PROVIDER_ERROR_TEXT}: internal error", "type": "server"}},
            )
        if mode == "400":
            return self._json(
                400,
                {
                    "error": {
                        "message": f"{PROVIDER_ERROR_TEXT}: bad request",
                        "type": "invalid_request_error",
                    }
                },
            )
        if mode == "hang":
            return self._hang(record)
        if not body.get("stream"):
            return self._complete(model)
        self._stream(model, body, broken=(mode == "break2"))

    def _hang(self, record: dict) -> None:
        deadline = time.monotonic() + HANG_LIMIT_S
        while time.monotonic() < deadline and not self.state.stop.is_set():
            if self._client_gone():
                record["client_disconnected"] = True
                break
            time.sleep(0.05)
        self.close_connection = True

    def _complete(self, model: str) -> None:
        self._json(
            200,
            {
                "id": f"chatcmpl-{int(time.time() * 1000)}",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "message": {"role": "assistant", "content": "".join(reply_words(model))},
                        "finish_reason": "stop",
                    }
                ],
                "usage": usage(),
            },
        )

    def _stream(self, model: str, body: dict, broken: bool) -> None:
        include_usage = bool((body.get("stream_options") or {}).get("include_usage"))
        cid = f"chatcmpl-{int(time.time() * 1000)}"
        created = int(time.time())
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Transfer-Encoding", "chunked")
        self.end_headers()
        try:
            self._sse(_chunk(cid, model, created, {"role": "assistant", "content": ""}, None))
            for index, word in enumerate(reply_words(model)):
                self._sse(_chunk(cid, model, created, {"content": word}, None))
                time.sleep(0.01)
                if broken and index == 1:
                    # Two content chunks went out: drop the socket without the final
                    # zero-length chunk, so the client sees a truncated body.
                    self.close_connection = True
                    self.connection.shutdown(socket.SHUT_RDWR)
                    return
            self._sse(_chunk(cid, model, created, {}, "stop"))
            if include_usage:
                final = _chunk(cid, model, created, {}, None)
                final["choices"] = []
                final["usage"] = usage()
                self._sse(final)
            self._sse("[DONE]")
            self.wfile.write(b"0\r\n\r\n")
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError, OSError):
            self.close_connection = True


def make_server(host: str = "127.0.0.1", port: int = 0) -> tuple[ThreadingHTTPServer, State]:
    state = State()
    handler = type("BoundHandler", (Handler,), {"state": state})
    server = ThreadingHTTPServer((host, port), handler)
    server.daemon_threads = True
    return server, state


def main(argv: list[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--host", default="0.0.0.0")  # noqa: S104 - container-local fake
    parser.add_argument("--port", type=int, default=8099)
    args = parser.parse_args(argv)
    server, state = make_server(args.host, args.port)
    print(f"llm-fake listening on {args.host}:{server.server_address[1]}", flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        state.stop.set()
        server.server_close()


if __name__ == "__main__":
    main()
