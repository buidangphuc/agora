"""Langfuse tracing of chat attempts (destructive: reconfigures team-ai).

The standing stack runs with Langfuse off. This scenario recreates team-ai with
compose/llm-fake-langfuse.override.yaml layered on the stack wrapper (Langfuse's ingestion
API is the fake's own `/api/public/ingestion`), and teardown recreates it without the
overlay and waits until chat answers again. If the Langfuse SDK exports asynchronously, the
assertion polls `GET /_ingested` rather than sleeping.
"""

from __future__ import annotations

import time

import httpx
from pytest_bdd import given, then, when

from tests.e2e.support import apr_support as apr
from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.world import World

OVERLAY = pe.REPO_ROOT / "platform-e2e" / "compose" / "llm-fake-langfuse.override.yaml"
EXPORT_WAIT_S = 45


def _x(world: World) -> dict:
    return world.state.extra


def _wait_chat(token: str, timeout_s: float = 120) -> None:
    deadline = time.monotonic() + timeout_s
    last = "no answer"
    while time.monotonic() < deadline:
        try:
            reply = apr.stream(token, "ready?", tagged=apr.tag(), timeout=15)
            if reply.ok and reply.deltas:
                return
            last = reply.describe()
        except httpx.HTTPError as exc:
            last = str(exc)
        time.sleep(1)
    raise TimeoutError(f"chat did not answer through the gateway after team-ai restart ({last})")


@given("team-ai runs with Langfuse enabled against the fake provider's ingestion endpoint")
def langfuse_on(world: World) -> None:
    x = _x(world)
    token = pe.register(world, "buyer")
    x["apr_token"] = token

    def restore() -> None:
        apr.dc("up", "-d", "--no-deps", "team-ai")
        pe.wait_healthy(pe.ai_container(), 120)
        _wait_chat(token)

    world.add_cleanup(restore)
    apr.fake_reset()
    apr.dc("-f", str(OVERLAY), "up", "-d", "--no-deps", "team-ai")
    pe.wait_healthy(pe.ai_container(), 120)
    env = apr.ai_env()
    assert (
        env.get("LANGFUSE_ENABLED", "").lower() == "true"
    ), "team-ai did not pick up LANGFUSE_ENABLED=true; is the wrapper applying the overlay?"
    _wait_chat(token)


@when('a buyer streams a chat message with a random X-Request-Id starting "e2e-trace-"')
def stream_with_trace_id(world: World) -> None:
    x = _x(world)
    x["apr_request_id"] = pe.rid("e2e-trace")
    x["apr_reply"] = apr.stream(
        x["apr_token"], "hello", request_id=x["apr_request_id"], tagged=apr.tag()
    )
    assert x["apr_reply"].ok, x["apr_reply"].describe()


@then("the fake ingestion endpoint receives a trace containing that request id and the buyer's id")
def trace_received(world: World) -> None:
    x = _x(world)
    rid, buyer = x["apr_request_id"], apr.jwt_subject(x["apr_token"])
    deadline = time.monotonic() + EXPORT_WAIT_S
    bodies: list[dict] = []
    while time.monotonic() < deadline:
        bodies = apr.fake_ingested(rid)
        if any(buyer in b["body"] for b in bodies):
            return
        time.sleep(1)
    seen = apr.fake_ingested("")
    raise AssertionError(
        f"no ingested body contains both {rid!r} and the buyer id {buyer!r}; "
        f"{len(bodies)} contain the request id, {len(seen)} bodies in total "
        f"({[b['path'] for b in seen][:5]})"
    )
