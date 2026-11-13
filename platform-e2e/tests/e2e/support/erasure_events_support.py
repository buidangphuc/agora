"""Black-box helpers for change commerce-user-erasure (initiative trust-legal).

Two things have no public RPC and are reached the way the stack lets a test reach them:

* `platform.identity.v1.UserErased` is produced to the real `identity.events` topic as a
  hand-encoded `EventEnvelope` (the same encoding `c2_identity_events` uses for SessionRevoked).
* `OrderService/GetUserObligations` and `PaymentService/GetPayoutObligations` are service-only and
  not routed by the gateway, so they are driven with `grpcurl` from a throwaway container on the
  stack network with the `x-principal-*` headers the interceptor reads (pattern of
  `flash_hold_support`).

Everything else (seeding, reading masked data) goes through the gateway. Names come from the
environment with the compose defaults:

    STACK_NETWORK              docker network of the stack (default agora_default)
    KAFKA_BROKERS              host-side broker (default localhost:9092; the local stack uses 19092)
    PLP_REDPANDA_CONTAINER     redpanda container used for `rpk group describe`
    ERASURE_ORDER_ADDR         default team-order-svc:50055
    ERASURE_PAYMENT_ADDR       default team-payment-svc:50056
    ERASURE_GRPCURL_IMAGE      default fullstorydev/grpcurl:latest
"""

from __future__ import annotations

import json
import os
import re
import time
import uuid
from pathlib import Path
from typing import Any

from tests.e2e.support import c2_identity_events as ide
from tests.e2e.support import plp_stack as stack

TOPIC = ide.TOPIC
USER_ERASED_TYPE = "platform.identity.v1.UserErased"
MASK = "Đã xóa"

ORDER_GROUP = "team-order.erasure"
PAYMENT_GROUP = "team-payment.erasure"
PROMOTION_GROUP = "team-promotion.erasure"
REFERRAL_GROUP = "team-referral-erasure"

ORDER_DLQ = "identity.events.order.dlq"
PAYMENT_DLQ = "identity.events.payment.dlq"
PROMOTION_DLQ = "identity.events.promotion.dlq"
REFERRAL_DLQ = "identity.events.referral.dlq"

CONSUME_WAIT_S = 40.0
POLL_S = 0.5

ORDER_OBLIGATIONS = ("order/v1/order.proto", "team-order", "platform.order.v1.OrderService")
PAYMENT_OBLIGATIONS = (
    "payment/v1/payment.proto",
    "team-payment",
    "platform.payment.v1.PaymentService",
)

PRINCIPALS: dict[str, tuple[str, str, str]] = {
    "service": ("service-team-identity", "service", ""),
    "user": ("", "user", ""),
    "service-no-scope": ("service-other", "service", "order.read"),
}


def repo_root() -> Path:
    return Path(__file__).resolve().parents[4]


def addr_of(service: str) -> str:
    if service == "team-order":
        return os.getenv("ERASURE_ORDER_ADDR", "team-order-svc:50055")
    return os.getenv("ERASURE_PAYMENT_ADDR", "team-payment-svc:50056")


def grpcurl_image() -> str:
    return os.getenv("ERASURE_GRPCURL_IMAGE", "fullstorydev/grpcurl:latest")


def _snake(grpc_code: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", grpc_code).lower()


def call_internal(
    target: tuple[str, str, str],
    method: str,
    body: dict[str, Any],
    principal: str | None,
    scope: str,
    principal_id: str = "",
) -> tuple[str, Any]:
    """One service-only RPC: (connect-style code, response dict or error text).

    `principal` is a key of PRINCIPALS or None for a call with no principal headers; `scope` is the
    scope a SERVICE principal carries (the right one for the RPC, or the one the case needs).
    """
    proto, repo, service = target
    pdir = repo_root() / repo / "proto"
    assert (pdir / "platform" / proto).is_file(), f"proto {proto} not found under {pdir}"
    cmd = [
        "run", "--rm", "--network", stack.stack_network(), "-v", f"{pdir}:/proto:ro",
        grpcurl_image(), "-plaintext", "-emit-defaults", "-import-path", "/proto",
        "-proto", f"platform/{proto}",
    ]  # fmt: skip
    if principal is not None:
        pid, ptype, default_scope = PRINCIPALS[principal]
        cmd += ["-H", f"x-principal-id: {principal_id or pid or 'e2e-user'}"]
        cmd += ["-H", f"x-principal-type: {ptype}"]
        use_scope = default_scope if principal == "service-no-scope" else scope
        if ptype == "service" and use_scope:
            cmd += ["-H", f"x-principal-scopes: {use_scope}"]
    cmd += ["-d", json.dumps(body), addr_of(repo), f"{service}/{method}"]
    proc = stack.docker(*cmd, check=False, timeout=120)
    if proc.returncode == 0:
        return "ok", json.loads(proc.stdout or "{}")
    err = (proc.stderr + proc.stdout).strip()
    m = re.search(r"Code:\s*(\w+)", err)
    return (_snake(m.group(1)) if m else f"grpcurl_exit_{proc.returncode}"), err


# ── UserErased events ────────────────────────────────────────────────────
def encode_user_erased(user_id: str, event_id: str, roles: tuple[str, ...] = ("buyer",)) -> bytes:
    payload = ide._ld(1, user_id.encode()) if user_id else b""
    for role in roles:
        payload += ide._ld(2, role.encode())
    payload += ide._ld(3, ide._timestamp(time.time()))
    return (
        ide._ld(1, event_id.encode())
        + ide._ld(2, USER_ERASED_TYPE.encode())
        + ide._ld(3, ide._timestamp(time.time()))
        + ide._ld(7, payload)
    )


def new_event_id() -> str:
    return f"e2e-erase-{uuid.uuid4()}"


def publish_user_erased(
    user_id: str, event_id: str | None = None, roles: tuple[str, ...] = ("buyer",)
) -> str:
    """Produce one UserErased envelope to identity.events (key = user id); returns its event id."""
    eid = event_id or new_event_id()
    stack.produce(TOPIC, key=user_id.encode(), value=encode_user_erased(user_id, eid, roles))
    return eid


def publish_garbage(marker: str) -> None:
    """A record on identity.events whose value is not an EventEnvelope; `marker` is its key."""
    stack.produce(TOPIC, key=marker.encode(), value=b"\xff\xfe\x00not-a-protobuf-envelope")


# ── consumer progress and DLQs ───────────────────────────────────────────
def wait_group_caught_up(group: str, timeout: float = CONSUME_WAIT_S) -> None:
    """The erasure consumer group exists and has no lag (it has handled what was produced)."""
    stack.wait_group_caught_up(group, timeout=timeout)


def topic_exists(topic: str) -> bool:
    out = stack.rpk("topic", "list", check=False).stdout
    return any(line.split()[:1] == [topic] for line in out.splitlines())


def assert_dlq_empty(topic: str, needle: bytes) -> None:
    assert topic_exists(topic), f"DLQ topic {topic} does not exist on the broker"
    found = stack.find_records(topic, needle, timeout_s=6.0, settle_s=1.0)
    assert not found, f"{topic} holds {len(found)} record(s) mentioning {needle!r}"


def wait_dlq_record(topic: str, needle: bytes, timeout: float = CONSUME_WAIT_S) -> list[dict]:
    assert topic_exists(topic), f"DLQ topic {topic} does not exist on the broker"
    deadline = time.monotonic() + timeout
    found: list[dict] = []
    while time.monotonic() < deadline:
        found = stack.find_records(topic, needle, timeout_s=4.0, settle_s=1.0)
        if found:
            return found
    return found


def wait_for(fn, ok, what: str, timeout: float = CONSUME_WAIT_S):
    """Poll `fn()` until `ok(value)`; returns the value, failing with `what` at the deadline."""
    deadline = time.monotonic() + timeout
    value = fn()
    while not ok(value) and time.monotonic() < deadline:
        time.sleep(POLL_S)
        value = fn()
    assert ok(value), f"{what} (last seen: {value!r})"
    return value
