"""Shared helpers for the ai-path-resilience e2e scenarios (area apr-e2e).

Everything goes through the public gateway (Connect streaming on ``ChatService/StreamChat``);
the only side channels are the two the spec names: the fake provider's own record of what it
received (``LLM_FAKE_URL``, published by compose/llm-fake.override.yaml) and team-ai's
container log (``AI_CONTAINER``).

Environment (defaults match the local ``agora`` stack):

    LLM_FAKE_URL     http://localhost:18099   the fake provider on the host
    AI_CONTAINER     agora-team-ai-svc        team-ai, for ``docker logs`` / ``docker inspect``
    DC_WRAPPER       the stack's compose wrapper (used by the tracing scenario)

Settings the scenarios depend on (timeouts, quota, limits, system prompt) are read from the
running team-ai container's environment, so the numbers in the overlay are not copied here.
"""

from __future__ import annotations

import base64
import json
import os
import re
import secrets
import string
import struct
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone

import httpx

from tests.e2e.support import pear_edge_support as pe

DEFAULT_FAKE_URL = "http://localhost:18099"
# What the fake appends after the "[<model>] " prefix (fakes/llm_fake/server.py REPLY_WORDS).
REPLY_TAIL = "xin chào đây là câu trả lời thử nghiệm"
PROVIDER_ERROR_TEXT = "fake-provider-secret-detail"
DEFAULT_SYSTEM_PROMPT = "You are the Agora shopping assistant. (e2e fixed system prompt)"
CHAIN = ("primary", "fb1", "fb2")
ALL_500 = "primary=500 fb1=500 fb2=500"
ALL_400 = "primary=400 fb1=400 fb2=400"
ASSISTANT = "/platform.ai.v1.AIService/ShoppingAssistant"
# Words that would betray an exception or a provider body in a client-visible error message.
LEAK_MARKERS = (
    PROVIDER_ERROR_TEXT,
    "traceback",
    "exception",
    "openai",
    "llm-fake",
    "internalservererror",
    "ratelimiterror",
    "http://",
    "status code",
)


def tag() -> str:
    """A letters-only token: it never trips the phone / ID redaction, and finds one message."""
    return "".join(secrets.choice(string.ascii_lowercase) for _ in range(14))


# ── the fake provider ────────────────────────────────────────────────────
def fake_url() -> str:
    return os.getenv("LLM_FAKE_URL", DEFAULT_FAKE_URL).rstrip("/")


def fake_requests(contains: str) -> list[dict]:
    resp = httpx.get(f"{fake_url()}/_requests", params={"contains": contains}, timeout=10)
    resp.raise_for_status()
    return sorted(resp.json()["requests"], key=lambda r: r["seq"])


def fake_models(contains: str) -> list[str]:
    return [r["model"] for r in fake_requests(contains)]


def fake_set_mode(**modes: str) -> None:
    httpx.post(f"{fake_url()}/_mode", json=modes, timeout=10).raise_for_status()


def fake_reset() -> None:
    httpx.post(f"{fake_url()}/_reset", timeout=10).raise_for_status()


def fake_ingested(contains: str) -> list[dict]:
    resp = httpx.get(f"{fake_url()}/_ingested", params={"contains": contains}, timeout=10)
    resp.raise_for_status()
    return resp.json()["ingested"]


# ── team-ai ──────────────────────────────────────────────────────────────
def ai_env() -> dict[str, str]:
    return pe.container_env(pe.ai_container())


def ai_setting(name: str, default: str) -> str:
    return ai_env().get(name) or default


def ai_int(name: str, default: int) -> int:
    return int(float(ai_setting(name, str(default))))


def ai_logs(since: datetime | None = None) -> str:
    args = ["logs"]
    if since is not None:
        args += ["--since", since.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")]
    out = pe.docker(*args, pe.ai_container(), check=False)
    return out.stdout + out.stderr


def log_start() -> datetime:
    """A `--since` anchor safely before the next call (covers small host/VM clock skew)."""
    return datetime.now(timezone.utc) - timedelta(seconds=5)


def jwt_subject(token: str) -> str:
    payload = token.split(".")[1]
    payload += "=" * (-len(payload) % 4)
    return str(json.loads(base64.urlsafe_b64decode(payload))["sub"])


# ── StreamChat ───────────────────────────────────────────────────────────
@dataclass
class Reply:
    sent: str
    session_id: str
    request_id: str | None
    http_status: int
    code: str
    message: str
    deltas: list[str]
    headers: httpx.Headers
    raw: bytes
    elapsed: float
    extra: dict = field(default_factory=dict)

    @property
    def text(self) -> str:
        return "".join(self.deltas)

    @property
    def norm(self) -> str:
        return " ".join(self.text.split())

    @property
    def ok(self) -> bool:
        return self.code == "ok"

    def describe(self) -> str:
        return (
            f"http={self.http_status} code={self.code} message={self.message!r} "
            f"reply={self.norm!r} raw={self.raw[:200]!r}"
        )


def _frames(raw: bytes):
    pos = 0
    while pos + 5 <= len(raw):
        flags = raw[pos]
        size = struct.unpack(">I", raw[pos + 1 : pos + 5])[0]
        body = raw[pos + 5 : pos + 5 + size]
        pos += 5 + size
        try:
            yield flags, (json.loads(body) if body else {})
        except ValueError:
            yield flags, {}


def error_message(raw: bytes, headers: httpx.Headers) -> str:
    """The Connect error message, from the end-stream envelope or a plain JSON error body."""
    if "application/connect+json" not in headers.get("content-type", ""):
        try:
            return str(json.loads(raw).get("message", ""))
        except ValueError:
            return ""
    for flags, data in _frames(raw):
        if flags & 0x02 and data.get("error"):
            return str(data["error"].get("message", ""))
    return ""


def stream(
    token: str,
    text: str = "hello",
    *,
    directive: str = "",
    session_id: str | None = None,
    request_id: str | None = None,
    tagged: str | None = None,
    timeout: float = 90,
) -> Reply:
    """One StreamChat call. `tagged` (a `tag()`) is appended so the fake's record can be found."""
    parts = [text]
    if tagged:
        parts.append(tagged)
    if directive:
        parts.append(f"[[fake {directive}]]")
    message = " ".join(parts)
    session = session_id or f"e2e-apr-{uuid.uuid4().hex[:10]}"
    headers = {"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"}
    headers["Authorization"] = f"bearer {token}"
    if request_id:
        headers["X-Request-Id"] = request_id
    payload = json.dumps({"sessionId": session, "message": message}).encode()
    body = b"\x00" + struct.pack(">I", len(payload)) + payload
    started = time.monotonic()
    resp = httpx.post(
        f"{pe.gateway_url()}{pe.CHAT_STREAM}", content=body, headers=headers, timeout=timeout
    )
    elapsed = time.monotonic() - started
    parsed = pe.parse_stream(resp.status_code, resp.headers, resp.content)
    return Reply(
        sent=message,
        session_id=session,
        request_id=request_id,
        http_status=resp.status_code,
        code=parsed.code,
        message=error_message(resp.content, resp.headers),
        deltas=parsed.deltas,
        headers=resp.headers,
        raw=resp.content,
        elapsed=elapsed,
        extra={"tag": tagged},
    )


def stream_when_available(token: str, text: str, deadline_s: float = 10, **kwargs) -> Reply:
    """Stream, retrying only while the answer is `unavailable`.

    The team-ai breakers are shared by every parallel scenario; a burst of other scenarios'
    injected failures can leave a target's breaker open for its short cooldown. A real
    quota or limit refusal (`resource_exhausted`) is never retried.
    """
    end = time.monotonic() + deadline_s
    while True:
        reply = stream(token, text, **kwargs)
        if reply.code != "unavailable" or time.monotonic() >= end:
            return reply
        time.sleep(0.5)


def answered_by(reply: Reply) -> str | None:
    match = re.match(r"\[(\w+)\] ", reply.text)
    return match.group(1) if match else None


def full_reply(model: str) -> str:
    return f"[{model}] {REPLY_TAIL}"


def assistant_call(token: str, message: str) -> httpx.Response:
    return pe.post_json(ASSISTANT, {"message": message}, token, timeout=60)


# ── compose wrapper ──────────────────────────────────────────────────────
def dc_wrapper() -> str:
    return os.getenv("DC_WRAPPER", pe.DC_WRAPPER_DEFAULT)


def dc(*args: str, timeout: int = 240) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [dc_wrapper(), *args], check=True, capture_output=True, text=True, timeout=timeout
    )
