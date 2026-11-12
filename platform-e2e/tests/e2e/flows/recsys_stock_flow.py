"""Search stock for recommendation scenarios (change recs-in-stock-filter).

`in_stock_only` placements read live stock from team-search, and an id the read-model does not know
fails closed. Catalogues that write synthetic listing ids straight into Qdrant or Redis therefore
also publish them to `listing.events` (the real search indexer path) with a stock, so team-search
projects it exactly as for a listing created through the gateway.

    QDRANT_URL                          serving Qdrant (default http://localhost:6333)
    RECS_STOCK_PRINCIPAL_SCOPES_ENV     team-ai variable that holds the scopes of its search principal
                                        (default RECS_SEARCH_PRINCIPAL_SCOPES)
    RECS_STOCK_AI_PORT                  team-ai gRPC port (default 50060)
"""

from __future__ import annotations

import os
import uuid
from collections.abc import Iterable
from typing import Any

import httpx

from config.settings import get_settings
from tests.e2e.flows import srm_events_flow as ev
from tests.e2e.support import pear_edge_support as pe

DEFAULT_STOCK = 5
SEEDED_SELLER = "e2e-recsys-stock-seller"
ITEM_ALIAS = "item_als_vectors"
_SCROLL = 1000


def qdrant_url() -> str:
    return os.getenv("QDRANT_URL", "http://localhost:6333")


def principal_scopes_env() -> str:
    return os.getenv("RECS_STOCK_PRINCIPAL_SCOPES_ENV", "RECS_SEARCH_PRINCIPAL_SCOPES")


def ai_port() -> str:
    return os.getenv("RECS_STOCK_AI_PORT", "50060")


def _publish_all(records: list[tuple[str, bytes]]) -> None:
    from confluent_kafka import Producer

    producer = Producer({"bootstrap.servers": ev.brokers(), "partitioner": "murmur2_random"})
    last: dict[int, int] = {}
    errors: list[Any] = []

    def _done(err, msg):  # noqa: ANN001
        if err is not None:
            errors.append(err)
        else:
            last[msg.partition()] = max(last.get(msg.partition(), -1), msg.offset())

    for key, value in records:
        producer.produce(ev.TOPIC, key=key.encode(), value=value, on_delivery=_done)
        producer.poll(0)
    producer.flush(60)
    assert not errors and last, f"publish to {ev.TOPIC} failed: {errors}"
    for partition, offset in last.items():
        ev.wait_consumed(partition, offset, 120.0)


def seed_stock(world, listing_ids: Iterable[str], stock: int = DEFAULT_STOCK) -> list[str]:
    """Publish each id as a published listing with `stock` and wait until the indexer consumed it.

    The ids are tombstoned in teardown so they do not linger in search."""
    ids = sorted(set(listing_ids))
    if not ids:
        return ids
    now = ev.now_ns()
    _publish_all(
        [
            (
                lid,
                ev.listing_changed(
                    ev.CREATED,
                    ev.listing_msg(
                        lid,
                        title=f"e2e recsys stock {lid}",
                        status=ev.PUBLISHED,
                        seller_id=SEEDED_SELLER,
                        stock=stock,
                    ),
                    now,
                ),
            )
            for lid in ids
        ]
    )
    if world is not None:
        world.add_cleanup(lambda: forget(ids))
    return ids


def forget(listing_ids: Iterable[str]) -> None:
    ids = sorted(set(listing_ids))
    now = ev.now_ns()
    _publish_all(
        [
            (
                lid,
                ev.listing_changed(
                    ev.DELETED,
                    ev.listing_msg(lid, status=ev.PUBLISHED, seller_id=SEEDED_SELLER),
                    now,
                ),
            )
            for lid in ids
        ]
    )


def set_stock(listing_ids: Iterable[str], stock: int) -> None:
    """Change the projected stock of already indexed listings (ListingStockChanged) and wait."""
    now = ev.now_ns()
    _publish_all([(lid, ev.stock_changed(lid, stock, now)) for lid in sorted(set(listing_ids))])


def serving_listing_ids() -> list[str]:
    """Every listing id of the serving item collection (the alias), empty when there is none."""
    found: list[str] = []
    offset: Any = None
    while True:
        body: dict[str, Any] = {"limit": _SCROLL, "with_payload": ["listing_id"]}
        if offset is not None:
            body["offset"] = offset
        resp = httpx.post(
            f"{qdrant_url()}/collections/{ITEM_ALIAS}/points/scroll", json=body, timeout=30.0
        )
        if resp.status_code == httpx.codes.NOT_FOUND:
            return found
        resp.raise_for_status()
        result = resp.json()["result"]
        for point in result["points"]:
            listing_id = (point.get("payload") or {}).get("listing_id")
            if listing_id:
                found.append(str(listing_id))
        offset = result.get("next_page_offset")
        if offset is None:
            return found


def seed_serving_stock() -> None:
    """Give every listing of the serving generation stock in search (live job runs rewrite it)."""
    seed_stock(None, serving_listing_ids())


def search_container() -> str:
    return get_settings().search_container


def stop_search() -> None:
    pe.docker("stop", search_container(), timeout=120)


def start_search() -> None:
    pe.docker("start", search_container(), check=False, timeout=120)


def wait_search_healthy(timeout_s: float = 90.0) -> None:
    from tests.e2e.flows.stack_flow import wait_for_search

    wait_for_search(get_settings().gateway_url, timeout_s)


def private_ai_without_search_scope(world) -> str:
    """Run one throwaway team-ai whose search principal lacks `search:read`; return its address.

    Same image and environment as the stack's team-ai, with the scopes variable of its search
    principal overridden to a scope that does not authorise GetListingStock."""
    source = pe.ai_container()
    env = pe.container_env(source)
    key = principal_scopes_env()
    assert key in env, (
        f"{source} has no {key}: set RECS_STOCK_PRINCIPAL_SCOPES_ENV to the variable that holds "
        "team-ai's search principal scopes"
    )
    env[key] = "ai.classify"
    image = pe.docker("inspect", source, "--format", "{{.Config.Image}}").stdout.strip()
    name = f"e2e-isr-ai-{uuid.uuid4().hex[:8]}"
    args = ["run", "-d", "--rm", "--name", name, "--network", pe.stack_network()]
    for k, v in env.items():
        if k not in {"PATH", "HOME", "HOSTNAME"}:
            args += ["-e", f"{k}={v}"]
    world.add_cleanup(lambda: pe.docker("rm", "-f", name, check=False))
    pe.docker(*args, image)
    pe.wait_healthy(name, 120.0)
    return f"{name}:{ai_port()}"
