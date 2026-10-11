"""Steps for the ranking dataset of recsys-gbdt-trainer (area rgd).

Black box: a buyer's impression, click and add-to-cart beacons go through the gateway edge
(POST /api/track) with one impressionId; the dataset job runs as the real image on the analytics volume
(rgd_flow). One batch is shared by the three real-stack scenarios (see `_shared`): the first creates it and
waits for the export cycle (PARQUET_EXPORT_INTERVAL_SECONDS, 300 s locally; override the wait with
RGD_EXPORT_WAIT_S). The AS_OF scenario feeds the image synthetic events.
"""

from __future__ import annotations

import os
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta

import pytest
from pytest_bdd import then, when

from tests.e2e.flows import fsm_job_flow as fsm
from tests.e2e.flows import rgd_flow as flow
from tests.e2e.support import tii_support as tii

EXPORT_WAIT_S = float(os.getenv("RGD_EXPORT_WAIT_S", "390"))
POLL_S = 10.0
AS_OF = "2026-10-09T12:00:00Z"
AS_OF_NAIVE = datetime(2026, 10, 9, 12, 0, 0)


def _naive(dt: datetime) -> str:
    return dt.strftime("%Y-%m-%dT%H:%M:%S")


@dataclass
class Shared:
    token: str
    user_id: str
    impression: str
    clicked: str
    carted: str
    ignored: str
    exported: bool = False


_SHARED: Shared | None = None


def _beacon(kind: str, s: dict, **extra) -> dict:
    return tii.view(
        f"e2e-rgd-{s['tag']}",
        type=kind,
        eventId=str(uuid.uuid4()),
        impressionId=s["impression"],
        placementId=f"e2e-rgd-placement-{s['tag']}",
        modelVersion=f"e2e-rgd-model-{s['tag']}",
        **extra,
    )


def _shared() -> Shared:
    """A new buyer is shown three listings (positions 1..3) in one row, clicks the first and adds the second."""
    global _SHARED
    if _SHARED is None:
        token, user_id = tii.register_buyer()
        tag = uuid.uuid4().hex[:8]
        s = {"tag": tag, "impression": f"e2e-rgd-imp-{tag}"}
        clicked, carted, ignored = (f"e2e-rgd-{tag}-{n}" for n in "abc")
        beacons = [
            _beacon("impression", s, listingId=lid, position=pos)
            for pos, lid in enumerate((clicked, carted, ignored), start=1)
        ]
        resp = tii.post_track(beacons, token)
        assert resp.status_code == 202, resp.text[:300]
        time.sleep(1.5)  # the outcomes must not precede the impressions
        for kind, lid in (("click", clicked), ("add_to_cart", carted)):
            resp = tii.post_track([_beacon(kind, s, listingId=lid)], token)
            assert resp.status_code == 202, resp.text[:300]
        _SHARED = Shared(token, user_id, s["impression"], clicked, carted, ignored)
    return _SHARED


def _wait_export(s: Shared) -> None:
    """Poll the analytics volume until the export holds the buyer's five events."""
    deadline = time.monotonic() + (0 if s.exported else EXPORT_WAIT_S)
    last: dict = {}
    while True:
        last = flow.probe_export([s.user_id])
        if last["users"].get(s.user_id, 0) >= 5:
            s.exported = True
            return
        if time.monotonic() >= deadline:
            raise AssertionError(f"the export did not hold the batch in {EXPORT_WAIT_S}s: {last}")
        time.sleep(POLL_S)


@dataclass
class Ctx:
    offline: object = None
    data: dict = field(default_factory=dict)
    inputs: list = field(default_factory=list)


@pytest.fixture
def rgd():
    c = Ctx(offline=fsm.new_offline_dir())
    yield c
    fsm.drop_offline_dir(c.offline)
    for p in c.inputs:
        flow.drop(p)


def _build_real(rgd: Ctx) -> Shared:
    s = _shared()
    _wait_export(s)
    r = flow.run_dataset(rgd.offline)
    assert r.exit_code == 0, f"dataset exited {r.exit_code}:\n{r.output[-3000:]}"
    rgd.data["shared"] = s
    rgd.data["rows"] = {
        row["listing_id"]: row for row in flow.dataset_rows(rgd.offline, [s.impression])["rows"]
    }
    return s


@when(
    "a buyer is shown listings in one recommendation row and clicks one of them with the same "
    "impressionId, the export cycle completes, and the dataset job runs"
)
def clicked_row(rgd):
    _build_real(rgd)


@then("rank_training@v1 has a row for that buyer, impression and listing with label 1")
def positive_row(rgd):
    s = rgd.data["shared"]
    row = rgd.data["rows"].get(s.clicked)
    assert row, f"no row for {s.clicked}: {rgd.data['rows']}"
    assert (row["user_key"], row["impression_id"], row["label"]) == (
        s.user_id,
        s.impression,
        1,
    ), row


@when(
    "the buyer adds another listing of that row to the cart with the same impressionId, the export "
    "cycle completes, and the dataset job runs"
)
def carted_row(rgd):
    _build_real(rgd)


@then("rank_training@v1 has a row for that listing with label 2")
def strong_positive_row(rgd):
    s = rgd.data["shared"]
    row = rgd.data["rows"].get(s.carted)
    assert row and row["label"] == 2, rgd.data["rows"]


@when("a third listing of that row is never clicked")
def ignored_row(rgd):
    _build_real(rgd)


@then("rank_training@v1 has a row for it with label 0 and the row's position")
def negative_row(rgd):
    s = rgd.data["shared"]
    row = rgd.data["rows"].get(s.ignored)
    assert row and (row["label"], row["position"]) == (0, 3), rgd.data["rows"]
    assert {r["position"] for r in rgd.data["rows"].values()} == {1, 2, 3}


@when(
    "the events hold one impression before AS_OF and one after it, and the dataset job runs with that AS_OF"
)
def as_of_events(rgd):
    tag = uuid.uuid4().hex[:8]
    before, after = f"rgd-before-{tag}", f"rgd-after-{tag}"
    rgd.data["ids"] = (before, after)
    events = [
        {"type": "impression", "listing": f"l-{before}", "user": "rgd-user", "impression": before,
         "position": 1, "at": _naive(AS_OF_NAIVE - timedelta(hours=2))},
        {"type": "impression", "listing": f"l-{after}", "user": "rgd-user", "impression": after,
         "position": 1, "at": _naive(AS_OF_NAIVE + timedelta(hours=2))},
    ]  # fmt: skip
    inputs = flow.make_inputs(events)
    rgd.inputs.append(inputs)
    r = flow.run_dataset(rgd.offline, as_of=AS_OF, inputs=inputs)
    assert r.exit_code == 0, r.output[-3000:]
    rgd.data["rows"] = flow.dataset_rows(rgd.offline, [before, after])


@then("rank_training@v1 has a row for the first and none for the second")
def as_of_rows(rgd):
    before, after = rgd.data["ids"]
    got = rgd.data["rows"]
    assert got["found"] and {r["impression_id"] for r in got["rows"]} == {before}, got
