"""Fixture Parquet inputs written with pyarrow, in the warehouse schemas (team-analytics warehouse.go)."""

from __future__ import annotations

import json
import shutil
from datetime import datetime, timedelta
from pathlib import Path

import fakeredis
import pyarrow as pa
import pyarrow.parquet as pq
import pytest

from featurestore.settings import Settings

AS_OF = datetime(2026, 10, 9, 12, 0, 0)
ROOT = Path(__file__).resolve().parent.parent

EVENTS = pa.schema(
    [
        ("event_id", pa.string()),
        ("event_type", pa.string()),
        ("listing_id", pa.string()),
        ("occurred_at", pa.timestamp("us")),
        ("ingested_at", pa.timestamp("us")),
        ("principal_type", pa.string()),
        ("user_key", pa.string()),
    ]
)
FACTS = pa.schema(
    [
        ("event_id", pa.string()),
        ("fact", pa.string()),
        ("user_id", pa.string()),
        ("listing_id", pa.string()),
        ("seller_id", pa.string()),
        ("rating", pa.int32()),
        ("occurred_at", pa.timestamp("us")),
        ("ingested_at", pa.timestamp("us")),
    ]
)
ORDERS = pa.schema(
    [
        ("event_id", pa.string()),
        ("order_id", pa.string()),
        ("listing_id", pa.string()),
        ("variant_id", pa.string()),
        ("seller_id", pa.string()),
        ("quantity", pa.int32()),
        ("unit_price", pa.int64()),
        ("currency", pa.string()),
        ("occurred_at", pa.timestamp("us")),
        ("status", pa.string()),
    ]
)


def write_inputs(d: Path, events=(), facts=(), orders=()):
    ev = [
        {
            "event_id": e["id"],
            "event_type": e["type"],
            "listing_id": e.get("listing"),
            "occurred_at": e["at"],
            "ingested_at": e.get("ing", e["at"]),
            "principal_type": e.get("ptype", "USER"),
            "user_key": e["user"],
        }
        for e in events
    ]
    fa = [
        {
            "event_id": f["id"],
            "fact": f["fact"],
            "user_id": f["user"],
            "listing_id": f.get("listing"),
            "seller_id": f.get("seller"),
            "rating": f.get("rating"),
            "occurred_at": f["at"],
            "ingested_at": f.get("ing", f["at"]),
        }
        for f in facts
    ]
    oa = [
        {
            "event_id": o["id"],
            "order_id": o["id"],
            "listing_id": o["listing"],
            "variant_id": "",
            "seller_id": "s",
            "quantity": 1,
            "unit_price": 1,
            "currency": "VND",
            "occurred_at": o["at"],
            "status": "PAID",
        }
        for o in orders
    ]
    pq.write_table(pa.Table.from_pylist(ev, schema=EVENTS), d / "tracking_events_resolved.parquet")
    pq.write_table(pa.Table.from_pylist(fa, schema=FACTS), d / "engagement_facts.parquet")
    pq.write_table(pa.Table.from_pylist(oa, schema=ORDERS), d / "order_facts.parquet")


def h(hours_before):
    return AS_OF - timedelta(hours=hours_before)


def d(days_before, extra_hours=0):
    return AS_OF - timedelta(days=days_before, hours=extra_hours)


@pytest.fixture
def dirs(tmp_path):
    (tmp_path / "in").mkdir()
    (tmp_path / "out").mkdir()
    return tmp_path / "in", tmp_path / "out"


@pytest.fixture
def redis():
    return fakeredis.FakeRedis(decode_responses=True)


@pytest.fixture
def env(dirs):
    i, o = dirs
    return {
        "FEATURESTORE_INPUT_DIR": str(i),
        "FEATURESTORE_OFFLINE_DIR": str(o),
        "FEATURESTORE_REDIS_URL": "redis://unused",
        "AS_OF": AS_OF.strftime("%Y-%m-%dT%H:%M:%SZ"),
    }


@pytest.fixture
def settings(env):
    return Settings.from_env(env)


@pytest.fixture
def registry_copy(tmp_path):
    dst = tmp_path / "registry"
    shutil.copytree(ROOT / "registry", dst)
    return dst


def manifest_of(out: Path, stamp: str) -> dict:
    return json.loads((out / "runs" / stamp / "manifest.json").read_text())
