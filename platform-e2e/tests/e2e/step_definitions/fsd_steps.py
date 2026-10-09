"""Steps for featurestore-datasets (area fsd-e2e).

Black box: tracking beacons through the gateway edge (`POST /api/track`), favourites through the
gateway RPCs, the dataset job (`python -m featurestore dataset`) and the recsys ALS job as the real
images on the stack network. The analytics export is observed on the analytics volume, the
dataset in a temp offline dir, the model registry in the stack Redis through the recsys job driver.
No scenario touches a container of the stack.

One batch of tracked activity is shared by the whole module (see `_shared`): the first scenario
that runs creates it and waits for the export cycle (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s
locally); later scenarios find the export already holding it. Override the wait with
FSD_EXPORT_WAIT_S. The dataset built "as of now" after the export is built once per module.
"""

from __future__ import annotations

import atexit
import os
import re
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path

import pytest
from pytest_bdd import given, parsers, then, when

from tests.e2e.flows import fsd_job_flow as fsd
from tests.e2e.flows.recsys_job_flow import run_recsys_job
from tests.e2e.support import efe_support as e
from tests.e2e.support import tii_support as tii
from tests.e2e.support.oic_order_support import Actor, OicWorld, register

EXPORT_WAIT_S = float(os.getenv("FSD_EXPORT_WAIT_S", "390"))
POLL_S = 10.0
DATASET_NAME = "als_interactions"
DATASET_VERSION = 1
MANIFEST_KEYS = {
    "name",
    "version",
    "as_of",
    "window_days",
    "rows",
    "users",
    "items",
    "definition_sha256",
    "input_watermark",
    "file_sha256",
    "file",
}


def _iso(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _parse(ts: str) -> datetime:
    dt = datetime.fromisoformat(str(ts).replace("Z", "+00:00"))
    return dt if dt.tzinfo else dt.replace(tzinfo=timezone.utc)


@dataclass
class Shared:
    world: OicWorld
    b1: Actor
    b2: Actor
    l1: str
    l2: str
    first_event_at: datetime
    exported: bool = False
    built: dict | None = None


_SHARED: Shared | None = None
_OFFLINE_DIRS: list[Path] = []


def _new_offline() -> Path:
    d = fsd.new_offline_dir()
    _OFFLINE_DIRS.append(d)
    return d


@atexit.register
def _cleanup() -> None:
    for d in _OFFLINE_DIRS:
        fsd.drop_offline_dir(d)


def _post_view(listing: str, token: str | None = None, **extra) -> None:
    resp = tii.post_track([tii.view(f"e2e-fsd-{tii.run_id()}", listingId=listing, **extra)], token)
    assert resp.status_code == 202 and resp.json().get("accepted") == 1, (
        resp.status_code,
        resp.text[:300],
    )


def _shared() -> Shared:
    """The shared activity, created once.

    b1: two views and a favourite of L1 (weight 2 x 1 + 3 = 5, 3 interactions).
    b2: an anonymous view of L2, then login with the same anonymous id and a second view (weight 2).
    """
    global _SHARED
    if _SHARED is None:
        w = OicWorld()
        e.seller_with_listings(w, 1)
        b1 = register(w, "b1", "buyer")
        b2 = register(w, "b2", "buyer")
        l1 = e.listing_id(w, "L1")
        l2 = f"e2e-fsd-{uuid.uuid4().hex[:10]}"
        first = datetime.now(timezone.utc)
        for _ in range(2):
            _post_view(l1, token=b1.token)
        e.favorite(w, b1, "L1")
        anon = f"e2e-anon-{uuid.uuid4()}"
        _post_view(l2, anonymousId=anon)
        _post_view(l2, token=b2.token, anonymousId=anon)
        _SHARED = Shared(w, b1, b2, l1, l2, first)
    return _SHARED


def _pairs(s: Shared) -> list[list[str]]:
    return [[s.b1.user_id, s.l1], [s.b2.user_id, s.l2]]


def _wait_export(s: Shared) -> None:
    """Poll the analytics volume until the export holds the shared activity."""
    if s.exported:
        return
    deadline = time.monotonic() + EXPORT_WAIT_S
    last: dict = {}
    while time.monotonic() < deadline:
        last = fsd.inspect("probe", pairs=_pairs(s))
        b1_views = last["resolved"].get(f"{s.b1.user_id}|{s.l1}", 0) >= 2
        b1_fact = last["facts"].get(f"{s.b1.user_id}|{s.l1}", 0) >= 1
        b2_views = last["resolved"].get(f"{s.b2.user_id}|{s.l2}", 0) >= 2
        if b1_views and b1_fact and b2_views:
            s.exported = True
            return
        time.sleep(POLL_S)
    raise AssertionError(
        f"the analytics export did not hold the activity in {EXPORT_WAIT_S}s: {last}"
    )


def _build(offline: Path, as_of: str) -> None:
    r = fsd.build_dataset(offline, as_of=as_of)
    assert r.exit_code == 0, f"dataset exited {r.exit_code}:\n{r.output[-3000:]}"


def _inspect_dataset(s: Shared, offline: Path) -> dict:
    return fsd.inspect("datasets", offline, users=[s.b1.user_id, s.b2.user_id])


def _built(s: Shared) -> dict:
    """Build once as of now (after the export holds the activity); cached for the module."""
    _wait_export(s)
    if s.built is None:
        offline = _new_offline()
        as_of = _iso(datetime.now(timezone.utc))
        _build(offline, as_of)
        s.built = {"offline": offline, "as_of": as_of, "inspect": _inspect_dataset(s, offline)}
    return s.built


def _only_file(inspected: dict) -> dict:
    files = inspected["files"]
    assert len(files) == 1, f"expected one dataset file, got {[f['path'] for f in files]}"
    return files[0]


@dataclass
class Ctx:
    shared: Shared | None = None
    data: dict = field(default_factory=dict)


@pytest.fixture
def fsd_ctx():
    return Ctx()


# ── Given ────────────────────────────────────────────────────────────────
@given(
    parsers.parse(
        'a new buyer "{name}" has viewed "{listing}" twice and favourited it, and the export '
        "cycle has completed"
    )
)
def buyer_exported(fsd_ctx, name, listing):
    s = _shared()
    _wait_export(s)
    fsd_ctx.shared = s


# ── scenario 1 ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a new buyer "{name}" views "{listing}" twice and favourites it, the export cycle '
        "completes, and the dataset is built"
    )
)
def weighted_row_built(fsd_ctx, name, listing):
    s = _shared()
    fsd_ctx.shared = s
    fsd_ctx.data["built"] = _built(s)


@then(
    parsers.parse(
        'the dataset has one row for "{name}" and "{listing}" with weight {weight:d} and '
        "{interactions:d} interactions"
    )
)
@then(parsers.parse('the dataset has one row for "{name}" and "{listing}" with weight {weight:d}'))
def one_row(fsd_ctx, name, listing, weight, interactions=None):
    s = fsd_ctx.shared
    buyer, lid = (s.b1, s.l1) if name == "b1" else (s.b2, s.l2)
    f = _only_file(fsd_ctx.data["built"]["inspect"])
    assert {"user_key", "listing_id", "weight", "interactions", "last_occurred_at"} <= set(
        f["columns"]
    ), f["columns"]
    rows = [r for r in f["users"][buyer.user_id] if r["listing_id"] == lid]
    assert len(rows) == 1, f"expected one row for {name}/{listing}, got {f['users']}"
    assert rows[0]["weight"] == weight, rows
    if interactions is not None:
        assert rows[0]["interactions"] == interactions, rows


# ── scenario 2 ───────────────────────────────────────────────────────────
@when(
    parsers.parse(
        'a visitor views "{listing}" anonymously, logs in as a new buyer "{name}" and views it '
        "again with the same anonymous id, the export cycle completes, and the dataset is built"
    )
)
def stitched_built(fsd_ctx, listing, name):
    s = _shared()
    fsd_ctx.shared = s
    fsd_ctx.data["built"] = _built(s)


# ── scenario 3 ───────────────────────────────────────────────────────────
@when(parsers.parse('the dataset is built with AS_OF just before "{name}"\'s first event'))
def built_before_first_event(fsd_ctx, name):
    s = fsd_ctx.shared
    # 5 s of margin for clock skew between the host and the stack; no shared event is before it.
    as_of = _iso(s.first_event_at - timedelta(seconds=5))
    offline = _new_offline()
    _build(offline, as_of)
    fsd_ctx.data["as_of"] = as_of
    fsd_ctx.data["inspect"] = _inspect_dataset(s, offline)


@then(parsers.parse('the dataset has no row for "{name}"'))
def no_row(fsd_ctx, name):
    s = fsd_ctx.shared
    f = _only_file(fsd_ctx.data["inspect"])
    assert f["manifest"], f"no manifest beside {f['path']}"
    assert _parse(f["manifest"]["as_of"]) == _parse(fsd_ctx.data["as_of"]), f["manifest"]
    assert f["users"][s.b1.user_id] == [], f"{name} has rows: {f['users'][s.b1.user_id]}"


# ── scenario 4 ───────────────────────────────────────────────────────────
@when("the dataset is built")
def dataset_built(fsd_ctx):
    s = fsd_ctx.shared
    fsd_ctx.data["built"] = _built(s)


@then(
    "its manifest names the as_of, a positive row count, and a file SHA-256 equal to the SHA-256 "
    "of the dataset file"
)
def manifest_describes_file(fsd_ctx):
    built = fsd_ctx.data["built"]
    f = _only_file(built["inspect"])
    m = f["manifest"]
    assert m, f"no as_of=*.manifest.json beside {f['path']}"
    assert MANIFEST_KEYS <= set(m), f"manifest keys {sorted(m)} lack {MANIFEST_KEYS - set(m)}"
    assert m["name"] == DATASET_NAME and m["version"] == DATASET_VERSION, m
    assert _parse(m["as_of"]) == _parse(built["as_of"]), m
    assert m["rows"] > 0 and m["rows"] == f["rows"], (m["rows"], f["rows"])
    assert re.fullmatch(r"[0-9a-f]{64}", m["file_sha256"]), m
    assert m["file_sha256"] == f["sha256"], f"manifest {m['file_sha256']} != file {f['sha256']}"
    assert re.fullmatch(r"[0-9a-f]{64}", m["definition_sha256"]), m


# ── scenario 5 ───────────────────────────────────────────────────────────
@when("the dataset is built and the recsys ALS job runs on it")
def recsys_on_dataset(fsd_ctx):
    s = fsd_ctx.shared
    built = _built(s)
    fsd_ctx.data["built"] = built
    # The job gets the offline dir read-only and finds the latest snapshot through DATASET_DIR.
    (run,) = run_recsys_job([], [{}], dataset_dir=built["offline"])
    assert run.exit_code == 0, f"the recsys job exited {run.exit_code}:\n{run.log}"
    fsd_ctx.data["run"] = run


@then(
    "the model the job registered records als_interactions, version 1, the dataset's as_of and "
    "the file SHA-256 from its manifest"
)
def model_names_dataset(fsd_ctx):
    run = fsd_ctx.data["run"]
    manifest = _only_file(fsd_ctx.data["built"]["inspect"])["manifest"]
    version = run.summary["model_version"]
    model = run.state["models"].get(version)
    assert model is not None, f"{version} not in registry: {run.state['models']}\n{run.summary}"
    lineage = (model["parameters"] or {}).get("dataset")
    assert lineage, f"parameters.dataset missing: {model['parameters']}"
    assert lineage["name"] == DATASET_NAME, lineage
    assert str(lineage["version"]) == str(DATASET_VERSION), lineage
    assert _parse(lineage["as_of"]) == _parse(manifest["as_of"]), (lineage, manifest["as_of"])
    assert lineage["sha256"] == manifest["file_sha256"], (lineage, manifest["file_sha256"])


# ── scenario 6 ───────────────────────────────────────────────────────────
@when("the recsys ALS job starts with DATASET_DIR pointing at an empty directory")
def recsys_without_dataset(fsd_ctx):
    # /work/empty-dataset is created empty by the driver; DATASET_PATH is unset so only DATASET_DIR
    # is consulted.
    (run,) = run_recsys_job(
        [],
        [{"DATASET_DIR": "/work/empty-dataset", "DATASET_PATH": None}],
        expect_summary=False,
    )
    fsd_ctx.data["run"] = run


@then("it exits non-zero, its log names DATASET_DIR, and no model is registered")
def refused(fsd_ctx):
    run = fsd_ctx.data["run"]
    assert run.exit_code != 0, f"the job trained without a dataset: {run.summary}\n{run.log}"
    assert "DATASET_DIR" in run.log, f"the log does not name DATASET_DIR:\n{run.log}"
    assert run.state["models"] == {}, run.state["models"]
    assert run.state["champion"] is None, run.state["champion"]
