"""StreamChat edge-policy steps (port-edge-authz-residuals, capability edge-stream-and-http-policy).

The stream is called black box through the gateway with the Connect streaming envelope over
HTTP (see tests/e2e/support/pear_edge_support.py). Connect reports a streaming error in the
end-stream envelope of an HTTP 200, or as a plain JSON error when the edge refuses before the
stream starts; the assertions read the Connect error code from either form, never a status
alone.

The rate-limit scenario needs a gateway whose burst can be exceeded by a test run. The
stack's gateway may be configured with a very large burst (compose: RATE_LIMIT_BURST=2000),
so when the stack's burst is above STACK_BURST_MAX the scenario starts its own gateway from
the same image and environment with the default limit (20 rps, burst 40) on the stack
network and calls that one; the real image is still the only thing under test.
"""

from __future__ import annotations

import json
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

import httpx
from pytest_bdd import given, parsers, then, when

from tests.e2e.support import pear_edge_support as pe
from tests.e2e.support.edge_tokens import sign_dev_token, user_claims
from tests.e2e.support.world import World

STACK_BURST_MAX = 200
PRIVATE_RPS, PRIVATE_BURST = 20, 40
OVERSIZED_BYTES = 20_000
_SKIP_ENV = {"PATH", "HOME", "HOSTNAME"}


def _x(world: World) -> dict:
    return world.state.extra


# ── Givens ───────────────────────────────────────────────────────────────
@given("a logged-in buyer for the stream checks")
@given("a freshly registered buyer for the stream checks")
def stream_buyer(world: World) -> None:
    _x(world)["pe_token"] = pe.register(world, "buyer")


@given("a gateway whose rate-limit burst is small enough to exceed")
def gateway_with_small_burst(world: World) -> None:
    x = _x(world)
    env = pe.container_env(pe.gateway_container())
    burst = int(env.get("RATE_LIMIT_BURST", PRIVATE_BURST))
    if burst <= STACK_BURST_MAX:
        x.update(pe_base=pe.gateway_url(), pe_burst=burst, pe_gw=pe.gateway_container())
        return
    name = f"e2e-edge-gw-{uuid.uuid4().hex[:8]}"
    image = pe.docker(
        "inspect", pe.gateway_container(), "--format", "{{.Config.Image}}"
    ).stdout.strip()
    args = ["run", "-d", "--rm", "--name", name, "--network", pe.stack_network()]
    args += ["-p", "127.0.0.1::8080"]
    private = {k: v for k, v in env.items() if k not in _SKIP_ENV}
    private.update(RATE_LIMIT_RPS=str(PRIVATE_RPS), RATE_LIMIT_BURST=str(PRIVATE_BURST))
    private["HTTP_PORT"] = "8080"
    for key, value in private.items():
        args += ["-e", f"{key}={value}"]
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    pe.docker(*args, image)
    port = pe.docker("port", name, "8080/tcp").stdout.strip().splitlines()[0].rsplit(":", 1)[1]
    base = f"http://127.0.0.1:{port}"
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        try:
            if httpx.get(f"{base}/healthz", timeout=2).status_code == httpx.codes.OK:
                break
        except httpx.HTTPError:
            pass
        time.sleep(0.5)
    else:
        raise TimeoutError(f"private gateway {name} did not become healthy")
    x.update(pe_base=base, pe_burst=PRIVATE_BURST, pe_gw=name)


# ── Whens ────────────────────────────────────────────────────────────────
@when(
    "a client calls StreamChat through the gateway with a bearer token whose signature is invalid"
)
def stream_with_invalid_signature(world: World) -> None:
    # A well-formed token from the local identity key with its signature replaced: only the
    # signature is wrong, so the refusal cannot be blamed on a malformed token.
    good = sign_dev_token(user_claims(f"e2e-{uuid.uuid4().hex[:8]}"))
    head, payload, sig = good.split(".")
    forged = f"{head}.{payload}.{sig[::-1]}"
    _x(world)["pe_stream"] = pe.stream_chat(forged)


@when(
    parsers.parse(
        'the buyer calls StreamChat through the gateway with a random X-Request-Id starting "{prefix}"'
    )
)
def stream_with_request_id(world: World, prefix: str) -> None:
    request_id = pe.rid(prefix.rstrip("-"))
    x = _x(world)
    x["pe_request_id"] = request_id
    x["pe_stream"] = pe.stream_chat(x["pe_token"], request_id=request_id)


@when("the buyer opens more StreamChat calls at once than the rate-limit burst")
def stream_flood(world: World) -> None:
    x = _x(world)
    total = x["pe_burst"] * 2
    with httpx.Client(limits=httpx.Limits(max_connections=total)) as client:
        with ThreadPoolExecutor(max_workers=total) as pool:
            futures = [
                pool.submit(pe.stream_chat, x["pe_token"], "hi", None, x["pe_base"], client)
                for _ in range(total)
            ]
            x["pe_streams"] = [f.result() for f in futures]


@when(parsers.parse("the buyer calls StreamChat with a {size:d}-byte message"))
def stream_oversized(world: World, size: int) -> None:
    x = _x(world)
    x["pe_stream"] = pe.stream_chat(x["pe_token"], "x" * size)


# ── Thens ────────────────────────────────────────────────────────────────
@then("the stream ends with the Connect error code unauthenticated and no chat message is streamed")
def refused_unauthenticated(world: World) -> None:
    result: pe.StreamResult = _x(world)["pe_stream"]
    assert result.code == "unauthenticated", (result.http_status, result.code, result.raw[:300])
    assert not result.streamed, f"a chat message was streamed: {result.deltas}"


@then(
    "the stream ends with the Connect error code resource_exhausted and no chat message is streamed"
)
def refused_resource_exhausted(world: World) -> None:
    result: pe.StreamResult = _x(world)["pe_stream"]
    assert result.code == "resource_exhausted", (result.http_status, result.code, result.raw[:300])
    assert not result.streamed, f"a chat message was streamed: {result.deltas}"


@then("the stream completes and the response carries that X-Request-Id")
def stream_echoes_request_id(world: World) -> None:
    x = _x(world)
    result: pe.StreamResult = x["pe_stream"]
    assert result.code == "ok" and result.streamed, (
        result.http_status,
        result.code,
        result.raw[:300],
    )
    assert result.headers.get("x-request-id") == x["pe_request_id"], dict(result.headers)


@then("at least one of them fails with resource_exhausted")
def some_resource_exhausted(world: World) -> None:
    results: list[pe.StreamResult] = _x(world)["pe_streams"]
    codes = sorted({r.code for r in results})
    assert "resource_exhausted" in codes, f"codes seen across {len(results)} streams: {codes}"
    # team-ai has its own limiter, so the refusal must be the gateway's: it logs one
    # edge.request line per stream with the final code.
    logs = pe.docker("logs", _x(world)["pe_gw"], check=False)
    refused = []
    for raw in (logs.stdout + logs.stderr).splitlines():
        try:
            rec = json.loads(raw)
        except ValueError:
            continue
        if (
            rec.get("msg") == "edge.request"
            and str(rec.get("method", "")).endswith("/StreamChat")
            and rec.get("code") == "resource_exhausted"
        ):
            refused.append(rec)
    assert (
        refused
    ), "the gateway logged no StreamChat edge.request line with code resource_exhausted"


@then("a different buyer's StreamChat call made right after succeeds")
def other_buyer_streams(world: World) -> None:
    x = _x(world)
    other = pe.register(world, "buyer")
    result = pe.stream_chat(other, "hello", base_url=x["pe_base"])
    assert result.code == "ok" and result.streamed, (
        result.http_status,
        result.code,
        result.raw[:300],
    )
