"""Black-box helpers for change domain-seller-erasure (initiative trust-legal).

`platform.identity.v1.UserErased` is produced to the real `identity.events` topic (encoder and consumer
progress helpers of erasure_events_support). The observable effects of the erasure are read through
the gateway (GetListing, search, GetStorefront, ListBundlesBySeller), the `listing.events` topic
(one ListingChanged DELETED envelope per deleted listing, the relay of the outbox rows) and, where no
RPC reaches it, listing_db:

    POSTGRES_CONTAINER   default agora-postgres-1
    DOMAIN_CONTAINER     default agora-team-domain-svc

listing_db is written only to seed states no public RPC can produce (held, banned, pending_review,
draft listings; 500 filler listings cloned from a real one).
"""

from __future__ import annotations

import os
import subprocess
import time
import uuid
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.flows.tracking_flow import _fields
from tests.e2e.support import c2_identity_events as ide
from tests.e2e.support import erasure_events_support as ev
from tests.e2e.support import plp_stack as stack
from tests.e2e.support import srm_support as s
from tests.e2e.support import user_enforcement_support as ue

LISTING = "/platform.listing.v1.ListingService/"
LISTING_EVENTS = "listing.events"
LISTING_CHANGED = "platform.listing.v1.ListingChanged"
CHANGE_DELETED = 3
ERASURE_GROUP = "team-domain-erasure"
ERASURE_DLQ = "identity.events.team-domain.dlq"
DOMAIN_PRINCIPAL = b"service-team-domain"
APPLY_S = 60.0

ue_ld = ide._ld


def psql(sql: str) -> str:
    container = os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")
    proc = subprocess.run(
        ["docker", "exec", container, "psql", "-U", "postgres", "-d", "listing_db",
         "-v", "ON_ERROR_STOP=1", "-At", "-c", sql],
        capture_output=True, text=True, timeout=90,
    )  # fmt: skip
    assert proc.returncode == 0, f"listing_db psql failed: {proc.stderr.strip()[:400]}"
    return proc.stdout.strip()


def lit(value: str) -> str:
    return "'" + value.replace("'", "''") + "'"


def count_listings(seller_id: str) -> int:
    return int(psql(f"SELECT count(*) FROM listings WHERE seller_id = {lit(seller_id)}"))


def count_variants(seller_id: str) -> int:
    return int(
        psql(
            "SELECT count(*) FROM listing_variants v JOIN listings l ON l.id::text = v.listing_id::text "
            f"WHERE l.seller_id = {lit(seller_id)}"
        )
    )


def count_rows(table: str, owner_column: str, owner_id: str) -> int:
    return int(psql(f"SELECT count(*) FROM {table} WHERE {owner_column} = {lit(owner_id)}"))


def set_status(listing_id: str, status: str) -> None:
    psql(f"UPDATE listings SET status = {lit(status)} WHERE id::text = {lit(listing_id)}")


def clone_listings(template_id: str, n: int) -> int:
    """`n` copies of a real listing (new ids, same seller) inserted straight into listing_db."""
    psql(
        "INSERT INTO listings SELECT (jsonb_populate_record(NULL::listings, to_jsonb(l) || "
        "jsonb_build_object('id', gen_random_uuid()::text))).* "
        f"FROM listings l, generate_series(1, {n}) WHERE l.id::text = {lit(template_id)}"
    )
    return n


def seller_listing_ids(seller_id: str) -> list[str]:
    out = psql(f"SELECT id::text FROM listings WHERE seller_id = {lit(seller_id)}")
    return out.splitlines() if out else []


def new_seller(prefix: str) -> dict:
    return ue.register_user("seller", prefix)


def create_listing(seller: dict, title: str | None = None, variants: int = 0) -> tuple[str, str]:
    """A published listing through the gateway; (id, title). `variants` adds that many variants."""
    title = title or f"Erasureprobe{uuid.uuid4().hex[:16]}"
    listing: dict[str, Any] = {
        "title": title,
        "description": "seller erasure e2e",
        "categoryId": "cat-laptop",
        "price": 100000,
        "stock": 3,
        "status": "LISTING_STATUS_PUBLISHED",
        "currency": "VND",
    }
    if variants:
        listing["variants"] = [
            {"name": f"V{i}", "sku": f"SKU-{uuid.uuid4().hex[:8]}", "price": 100000, "stock": 2}
            for i in range(variants)
        ]
    resp = s.post(LISTING + "CreateListing", {"listing": listing}, seller["token"])
    assert resp.status_code == 200, f"CreateListing {resp.status_code}: {resp.text}"
    return resp.json()["listing"]["id"], title


def get_listing(listing_id: str, token: str | None = None) -> httpx.Response:
    return s.post(LISTING + "GetListing", {"id": listing_id}, token)


def search_hits(query: str) -> list[str]:
    resp = s.post("/platform.search.v1.SearchService/SearchListings", {"query": query})
    assert resp.status_code == 200, f"SearchListings {resp.status_code}: {resp.text}"
    return [h["listingId"] for h in resp.json().get("hits", [])]


def upsert_storefront(seller: dict, slug: str) -> None:
    resp = s.post(
        LISTING + "UpsertStorefront",
        {"storefront": {"slug": slug, "tagline": "erasure e2e", "displayName": f"Shop {slug}"}},
        seller["token"],
    )
    assert resp.status_code == 200, f"UpsertStorefront {resp.status_code}: {resp.text}"


def storefront_code(slug: str) -> int:
    return s.post(LISTING + "GetStorefront", {"slug": slug}).status_code


def create_bundle(seller: dict, listing_ids: list[str], title: str) -> None:
    resp = s.post(
        LISTING + "CreateBundle",
        {"title": title, "listingIds": listing_ids, "bundlePrice": 150000},
        seller["token"],
    )
    assert resp.status_code == 200, f"CreateBundle {resp.status_code}: {resp.text}"


def bundles_of(seller_id: str) -> list[dict]:
    resp = s.post(LISTING + "ListBundlesBySeller", {"sellerId": seller_id})
    assert resp.status_code == 200, f"ListBundlesBySeller {resp.status_code}: {resp.text}"
    return resp.json().get("bundles", [])


def _text(value: Any) -> str:
    return (
        bytes(value).decode("utf-8", errors="replace") if isinstance(value, bytes) else str(value)
    )


def deleted_envelopes(since_ms: int, timeout_s: float = 20.0) -> dict[str, list[dict]]:
    """listing id -> every ListingChanged(DELETED) envelope on listing.events since `since_ms`."""
    from confluent_kafka import Consumer, TopicPartition

    consumer = Consumer(
        {
            "bootstrap.servers": get_settings().kafka_brokers,
            "group.id": f"e2e-sel-{uuid.uuid4().hex}",
            "enable.auto.commit": False,
        }
    )
    found: dict[str, list[dict]] = {}
    try:
        meta = consumer.list_topics(LISTING_EVENTS, timeout=10).topics.get(LISTING_EVENTS)
        if meta is None or meta.error is not None:
            return found
        starts, ends = [], {}
        for p in meta.partitions:
            low, high = consumer.get_watermark_offsets(TopicPartition(LISTING_EVENTS, p), 10)
            resolved = consumer.offsets_for_times(
                [TopicPartition(LISTING_EVENTS, p, since_ms)], 10
            )[0]
            if high <= low or resolved.offset < 0:
                continue
            starts.append(resolved)
            ends[p] = high
        if not starts:
            return found
        consumer.assign(starts)
        deadline = time.monotonic() + timeout_s
        while ends and time.monotonic() < deadline:
            msg = consumer.poll(0.5)
            if msg is None or msg.error():
                continue
            if msg.offset() + 1 >= ends.get(msg.partition(), 0):
                ends.pop(msg.partition(), None)
            env = decode_envelope(msg.value() or b"")
            if env["type"] == LISTING_CHANGED and env["change_type"] == CHANGE_DELETED:
                env["raw"] = msg.value() or b""
                found.setdefault(env["listing_id"], []).append(env)
    finally:
        consumer.close()
    return found


def decode_envelope(raw: bytes) -> dict[str, Any]:
    out: dict[str, Any] = {"type": "", "listing_id": "", "change_type": 0, "principal": b""}
    payload = b""
    for number, wire, value in _fields(raw):
        if number == 2 and wire == 2:
            out["type"] = _text(value)
        elif number == 4 and wire == 2:
            out["principal"] = bytes(value)  # type: ignore[arg-type]
        elif number == 7 and wire == 2:
            payload = bytes(value)  # type: ignore[arg-type]
    if out["type"] == LISTING_CHANGED:
        for number, wire, value in _fields(payload):
            if number == 1 and wire == 2:
                for n2, w2, v2 in _fields(bytes(value)):  # type: ignore[arg-type]
                    if n2 == 1 and w2 == 2:
                        out["listing_id"] = _text(v2)
            elif number == 2 and wire == 0:
                out["change_type"] = int(value)  # type: ignore[arg-type]
    return out


def wait_deleted(
    ids: list[str], since_ms: int, timeout_s: float = APPLY_S
) -> dict[str, list[dict]]:
    deadline = time.monotonic() + timeout_s
    got: dict[str, list[dict]] = {}
    while time.monotonic() < deadline:
        got = deleted_envelopes(since_ms, timeout_s=15.0)
        if all(i in got for i in ids):
            return got
        time.sleep(1.0)
    missing = [i for i in ids if i not in got]
    raise AssertionError(
        f"{len(missing)} of {len(ids)} listings have no DELETED event: {missing[:3]}"
    )


def encode_user_erased_without_erased_at(user_id: str, event_id: str) -> bytes:
    """A UserErased envelope whose payload has user_id and roles but no erased_at."""
    payload = ue_ld(1, user_id.encode()) + ue_ld(2, b"seller")
    return (
        ue_ld(1, event_id.encode())
        + ue_ld(2, ev.USER_ERASED_TYPE.encode())
        + ue_ld(3, ide._timestamp(time.time()))
        + ue_ld(7, payload)
    )


def domain_logs(since_s: int) -> str:
    container = os.getenv("DOMAIN_CONTAINER", "agora-team-domain-svc")
    proc = stack.docker("logs", "--since", f"{since_s}s", container, check=False, timeout=60)
    return proc.stdout + proc.stderr


def produce_status_changed_via_admin(world: Any, user_id: str, status: str) -> None:
    resp = ue.set_user_status(world, user_id, status, "scam", ue.in_future(3600))
    assert resp.status_code == 200, f"SetUserStatus failed: {resp.status_code} {resp.text}"
