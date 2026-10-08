"""Shared helpers for the port-edge-authz-residuals edge scenarios (area pear-edge).

Black box through the public gateway: Connect streaming calls are made with the Connect
streaming envelope over plain HTTP (content type `application/connect+json`, each message
framed as 1 flag byte + 4 byte big-endian length + JSON), so no generated client is needed.

Container, image and wrapper names come from the environment with the local `agora`
compose defaults:

    GATEWAY_CONTAINER      agora-team-gateway-svc
    AI_CONTAINER           agora-team-ai-svc
    PAYMENT_CONTAINER      agora-team-payment-svc
    GATEWAY_BOOT_IMAGE     agora-team-gateway:local
    IDENTITY_BOOT_IMAGE    agora-team-identity:local   (falls back to IDENTITY_IMAGE)
    DC_WRAPPER             the stack's compose wrapper script
"""

from __future__ import annotations

import json
import os
import struct
import subprocess
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path

import httpx

from config.settings import get_settings

PASSWORD = "Sup3r-secret-pass!"
CHAT_STREAM = "/platform.chat.v1.ChatService/StreamChat"
REPO_ROOT = Path(__file__).resolve().parents[4]
DC_WRAPPER_DEFAULT = "/Users/phuc.buidang/Library/Caches/ai-first-runs/agora-stack/dc-agora-ov.sh"
_DOCKER_TIMEOUT_S = 120


# ── names ────────────────────────────────────────────────────────────────
def gateway_container() -> str:
    return os.getenv("GATEWAY_CONTAINER", "agora-team-gateway-svc")


def ai_container() -> str:
    return os.getenv("AI_CONTAINER", "agora-team-ai-svc")


def payment_container() -> str:
    return os.getenv("PAYMENT_CONTAINER", "agora-team-payment-svc")


def gateway_boot_image() -> str:
    return os.getenv("GATEWAY_BOOT_IMAGE", "agora-team-gateway:local")


def identity_boot_image() -> str:
    return os.getenv(
        "IDENTITY_BOOT_IMAGE", os.getenv("IDENTITY_IMAGE", "agora-team-identity:local")
    )


def stack_network() -> str:
    return get_settings().stack_network


def gateway_url() -> str:
    return get_settings().gateway_url.rstrip("/")


def rid(prefix: str) -> str:
    """A request id the gateway accepts as well-formed (`[A-Za-z0-9._-]`)."""
    return f"{prefix}-{uuid.uuid4().hex[:12]}"


# ── docker ───────────────────────────────────────────────────────────────
def docker(*args: str, check: bool = True, timeout: int = _DOCKER_TIMEOUT_S):
    return subprocess.run(
        ["docker", *args], capture_output=True, text=True, timeout=timeout, check=check
    )


def container_env(name: str) -> dict[str, str]:
    out = docker("inspect", name, "--format", "{{json .Config.Env}}").stdout
    env: dict[str, str] = {}
    for item in json.loads(out) or []:
        key, _, value = item.partition("=")
        env[key] = value
    return env


def compose_env(service: str) -> dict[str, str]:
    """The environment the root compose file gives `service` (KEY=VALUE list entries)."""
    import yaml

    compose = yaml.safe_load((REPO_ROOT / "docker-compose.services.yaml").read_text())
    entries = compose["services"][service]["environment"]
    return dict(e.split("=", 1) for e in entries if isinstance(e, str) and "=" in e)


def wait_healthy(name: str, timeout_s: float = 120.0) -> None:
    """Block until the container is running (and healthy when it declares a healthcheck)."""
    deadline = time.monotonic() + timeout_s
    state = "unknown"
    while time.monotonic() < deadline:
        out = docker(
            "inspect",
            name,
            "--format",
            "{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}",
            check=False,
        )
        state = out.stdout.strip() or out.stderr.strip()
        status, _, health = state.partition(" ")
        if status == "running" and health in ("healthy", "none"):
            return
        time.sleep(1)
    raise TimeoutError(f"{name} did not become healthy within {timeout_s}s ({state})")


# ── users ────────────────────────────────────────────────────────────────
def register(world, role: str) -> str:
    username = f"e2e_{role}_{uuid.uuid4().hex[:10]}"
    token = world.service_factory.auth.register(username, PASSWORD, role=role)
    assert token, f"could not register a {role}"
    return token


# ── plain Connect JSON ───────────────────────────────────────────────────
def post_json(
    path: str,
    body: dict,
    token: str | None = None,
    headers: dict[str, str] | None = None,
    base_url: str | None = None,
    timeout: float = 30,
) -> httpx.Response:
    hdrs = {"Content-Type": "application/json", **(headers or {})}
    if token:
        hdrs["Authorization"] = f"bearer {token}"
    return httpx.post(
        f"{base_url or gateway_url()}{path}", json=body, headers=hdrs, timeout=timeout
    )


def connect_code(resp: httpx.Response) -> str:
    """Connect error code of a unary response (``ok`` for a 2xx)."""
    if resp.status_code < 400:
        return "ok"
    try:
        return str(resp.json().get("code", f"http_{resp.status_code}"))
    except ValueError:
        return f"http_{resp.status_code}"


# ── Connect server streaming over HTTP ───────────────────────────────────
@dataclass
class StreamResult:
    """What a `StreamChat` call returned on the wire.

    Connect carries a streaming error in the end-stream envelope (flag 0x02) of an HTTP 200,
    or, when the edge refuses before streaming starts, as a plain JSON error body with the
    mapped HTTP status; `code` is the Connect error code from whichever form was used.
    """

    http_status: int
    headers: httpx.Headers
    deltas: list[str] = field(default_factory=list)
    code: str = "ok"
    raw: bytes = b""

    @property
    def streamed(self) -> bool:
        return bool(self.deltas)


def _envelope(message: dict) -> bytes:
    payload = json.dumps(message).encode()
    return b"\x00" + struct.pack(">I", len(payload)) + payload


def parse_stream(resp_status: int, headers: httpx.Headers, raw: bytes) -> StreamResult:
    result = StreamResult(http_status=resp_status, headers=headers, raw=raw)
    if "application/connect+json" not in headers.get("content-type", ""):
        try:
            result.code = str(json.loads(raw).get("code", f"http_{resp_status}"))
        except ValueError:
            result.code = "ok" if resp_status < 400 else f"http_{resp_status}"
        return result
    pos = 0
    while pos + 5 <= len(raw):
        flags = raw[pos]
        size = struct.unpack(">I", raw[pos + 1 : pos + 5])[0]
        body = raw[pos + 5 : pos + 5 + size]
        pos += 5 + size
        try:
            data = json.loads(body) if body else {}
        except ValueError:
            data = {}
        if flags & 0x02:  # end-stream
            error = data.get("error")
            if error:
                result.code = str(error.get("code", "unknown"))
        elif data.get("delta"):
            result.deltas.append(data["delta"])
    return result


def stream_chat(
    token: str | None,
    message: str = "hello",
    request_id: str | None = None,
    base_url: str | None = None,
    client: httpx.Client | None = None,
    timeout: float = 60,
) -> StreamResult:
    headers = {"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"}
    if token:
        headers["Authorization"] = f"bearer {token}"
    if request_id:
        headers["X-Request-Id"] = request_id
    url = f"{base_url or gateway_url()}{CHAT_STREAM}"
    body = _envelope({"sessionId": f"e2e-{uuid.uuid4().hex[:8]}", "message": message})
    http = client or httpx
    resp = http.post(url, content=body, headers=headers, timeout=timeout)
    return parse_stream(resp.status_code, resp.headers, resp.content)
