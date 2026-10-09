"""Helpers for engagement-fact-events (area efe-e2e).

Black box: engagement RPCs go through the gateway (Connect JSON, via the oic helpers); facts are
observed on `engagement.events` (binary EventEnvelope, decoded without stubs) and in the
team-analytics warehouse (`tii_support.warehouse_rows`). The Kafka-down scenario stops and starts
redpanda through the compose wrapper named by DC_WRAPPER.
"""

from __future__ import annotations

import os
import subprocess
import time

from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import plp_stack as stack
from tests.e2e.support.oic_order_support import (
    COMPLETED,
    Actor,
    OicWorld,
    create_listing,
    ok,
    place_order,
    post,
    put_in_status,
    register,
)

ENGAGEMENT = "/platform.engagement.v1.EngagementService"
TOPIC = "engagement.events"
DLQ_TOPIC = "engagement.events.analytics.dlq"
TYPE_PREFIX = "platform.engagement.v1."
TYPES = {
    "FavoriteAdded": {"user_id": 1, "listing_id": 2},
    "FavoriteRemoved": {"user_id": 1, "listing_id": 2},
    "SellerFollowed": {"user_id": 1, "seller_id": 2},
    "SellerUnfollowed": {"user_id": 1, "seller_id": 2},
    "ReviewCreated": {"review_id": 1, "user_id": 2, "listing_id": 3, "seller_id": 4, "rating": 5},
}
DEFAULT_WRAPPER = os.path.expanduser("~/Library/Caches/ai-first-runs/agora-stack/dc-agora-ov.sh")


def _text(v) -> str:
    return bytes(v).decode("utf-8") if isinstance(v, (bytes, bytearray)) else ""


def decode(record: dict) -> dict:
    """Decode a record from `plp_stack.find_records` into type/payload/principal/key/offset."""
    out: dict = {"key": record["key"].decode("utf-8", "replace"), "offset": record["offset"]}
    out["raw"] = record["value"]
    out["type"] = ""
    out["principal_id"] = ""
    out["payload"] = {}
    payload = b""
    for number, wire, value in _fields(record["value"]):
        if number == 2 and wire == 2:
            out["type"] = _text(value)
        elif number == 4 and wire == 2:
            for n, w, v in _fields(bytes(value)):  # type: ignore[arg-type]
                if n == 1 and w == 2:
                    out["principal_id"] = _text(v)
        elif number == 7 and wire == 2:
            payload = bytes(value)  # type: ignore[arg-type]
    short = out["type"].removeprefix(TYPE_PREFIX)
    names = {n: name for name, n in TYPES.get(short, {}).items()}
    for number, wire, value in _fields(payload):
        name = names.get(number, f"f{number}")
        out["payload"][name] = _text(value) if wire == 2 else value
    out["payload_bytes"] = payload
    return out


def facts(needle: str, kind: str | None = None, timeout_s: float = 20.0) -> list[dict]:
    """Decoded envelopes on engagement.events mentioning `needle`, oldest first, optional type."""
    recs = stack.find_records(TOPIC, needle.encode(), timeout_s=timeout_s, settle_s=2.0)
    decoded = sorted((decode(r) for r in recs), key=lambda d: d["offset"])
    if kind:
        decoded = [d for d in decoded if d["type"] == TYPE_PREFIX + kind]
    return decoded


def wait_facts(
    needle: str, kind: str, count: int = 1, timeout_s: float = 90.0, settle_s: float = 4.0
) -> list[dict]:
    """Wait until `count` envelopes of `kind` exist, then re-scan after a pause: still `count`."""
    deadline = time.monotonic() + timeout_s
    got: list[dict] = []
    while time.monotonic() < deadline:
        got = facts(needle, kind, timeout_s=6.0)
        if len(got) >= count:
            time.sleep(settle_s)
            got = facts(needle, kind, timeout_s=6.0)
            if len(got) == count:
                return got
            raise AssertionError(f"expected exactly {count} {kind} for {needle}, saw {len(got)}")
        time.sleep(1)
    raise AssertionError(f"expected {count} {kind} for {needle} on {TOPIC}, saw {len(got)}")


# ── actors ───────────────────────────────────────────────────────────────
def new_world() -> OicWorld:
    return OicWorld()


def seller_with_listings(w: OicWorld, n: int = 1) -> Actor:
    seller = register(w, "seller", "seller")
    for i in range(n):
        create_listing(w, f"L{i + 1}", seller, 50)
    return seller


def listing_id(w: OicWorld, name: str) -> str:
    return w.listings[name]["id"]


def favorite(w: OicWorld, buyer: Actor, listing: str) -> None:
    ok(post(w, buyer, ENGAGEMENT, "AddFavorite", {"listingId": listing_id(w, listing)}))


def unfavorite(w: OicWorld, buyer: Actor, listing: str) -> None:
    ok(post(w, buyer, ENGAGEMENT, "RemoveFavorite", {"listingId": listing_id(w, listing)}))


def follow(w: OicWorld, buyer: Actor, seller: Actor) -> None:
    ok(post(w, buyer, ENGAGEMENT, "FollowSeller", {"sellerId": seller.user_id}))


def delivered_order(w: OicWorld, buyer: Actor, seller: Actor, listing: str) -> str:
    order = place_order(w, buyer, listing, 1)
    put_in_status(w, order, buyer, seller, COMPLETED)
    return order


def review(w: OicWorld, buyer: Actor, listing: str, rating: int, comment: str, order: str) -> None:
    ok(
        post(
            w,
            buyer,
            ENGAGEMENT,
            "CreateReview",
            {
                "listingId": listing_id(w, listing),
                "rating": rating,
                "comment": comment,
                "orderId": order,
            },
        )
    )


# ── redpanda lifecycle (destructive lane) ────────────────────────────────
def wrapper() -> str:
    return os.getenv("DC_WRAPPER", DEFAULT_WRAPPER)


def _dc(*args: str) -> None:
    res = subprocess.run([wrapper(), *args], capture_output=True, text=True, timeout=180)
    assert res.returncode == 0, f"{wrapper()} {' '.join(args)} failed: {res.stderr.strip()[:300]}"


def stop_redpanda() -> None:
    _dc("stop", "redpanda")


def start_redpanda_and_settle() -> None:
    _dc("start", "redpanda")
    # wait_settled reads PLP_<SVC>_CONTAINER; point it at the real redpanda container.
    os.environ.setdefault("PLP_REDPANDA_CONTAINER", stack.redpanda_container())
    deadline = time.monotonic() + 120
    while time.monotonic() < deadline:
        if stack.rpk("cluster", "info", check=False, timeout=15).returncode == 0:
            break
        time.sleep(2)
    stack.wait_settled("redpanda")
