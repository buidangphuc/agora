"""AI single-attempt and reflection-off steps (port-edge-authz-residuals).

MagicListing is called through the gateway while the real team-ai container is stopped; the
attempt count is read from the gateway's own `edge.request` JSON log line for the request id.
"""

from __future__ import annotations

import json
import struct
import time

import httpx
from pytest_bdd import given, parsers, then, when

from config.settings import get_settings
from tests.e2e.flows import stop_container
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

MAGIC = "/platform.ai.v1.AIService/MagicListing"


def _x(world: World) -> dict:
    return world.state.extra


@given("a logged-in seller for the edge AI checks")
def ai_seller(world: World) -> None:
    _x(world)["pe_token"] = pe.register(world, "seller")


@given("team-ai is stopped for the edge AI checks")
def stop_team_ai(world: World) -> None:
    token = _x(world)["pe_token"]
    name = pe.ai_container()
    restore = stop_container(name)

    def restore_and_wait() -> None:
        restore()
        pe.wait_healthy(name, 120)
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            try:
                if pe.post_json(MAGIC, {"titleHint": "restore probe"}, token).status_code == 200:
                    return
            except httpx.HTTPError:
                pass
            time.sleep(1)
        raise TimeoutError("MagicListing did not recover through the gateway after team-ai restart")

    world.add_cleanup(restore_and_wait)


@when(
    parsers.parse(
        'the seller calls MagicListing through the gateway with a random X-Request-Id starting "{prefix}"'
    )
)
def call_magic(world: World, prefix: str) -> None:
    x = _x(world)
    x["pe_request_id"] = pe.rid(prefix.rstrip("-"))
    x["pe_started"] = time.time() - 5
    x["pe_resp"] = pe.post_json(
        MAGIC,
        {"titleHint": "Laptop Dell XPS 13"},
        x["pe_token"],
        headers={"X-Request-Id": x["pe_request_id"]},
        timeout=60,
    )


@then("the call fails with unavailable")
def call_unavailable(world: World) -> None:
    resp: httpx.Response = _x(world)["pe_resp"]
    assert resp.status_code == 503 and pe.connect_code(resp) == "unavailable", (
        resp.status_code,
        resp.text[:300],
    )


@then("the gateway's edge.request log line for that request id reports one upstream attempt")
def one_attempt(world: World) -> None:
    x = _x(world)
    since = f"{int(time.time() - x['pe_started']) + 30}s"
    deadline = time.monotonic() + 15
    line = None
    while time.monotonic() < deadline and line is None:
        out = pe.docker("logs", "--since", since, pe.gateway_container(), check=False)
        for raw in (out.stdout + out.stderr).splitlines():
            try:
                rec = json.loads(raw)
            except ValueError:
                continue
            if rec.get("msg") == "edge.request" and rec.get("request_id") == x["pe_request_id"]:
                line = rec
        if line is None:
            time.sleep(1)
    assert line is not None, f"no edge.request line for {x['pe_request_id']}"
    assert line.get("attempts") == 1, line


# ── reflection ───────────────────────────────────────────────────────────
@when(
    "a client posts a reflection request to /grpc.reflection.v1.ServerReflection/ServerReflectionInfo "
    "on the local gateway"
)
def post_reflection(world: World) -> None:
    body = b"\x00" + struct.pack(">I", 2) + b"{}"
    base = get_settings().gateway_url.rstrip("/")
    _x(world)["pe_resp"] = httpx.post(
        f"{base}/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
        content=body,
        headers={"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"},
        timeout=15,
    )


@then("the gateway answers HTTP 404")
def answers_404(world: World) -> None:
    resp: httpx.Response = _x(world)["pe_resp"]
    assert resp.status_code == 404, (resp.status_code, resp.text[:200])
