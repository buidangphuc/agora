"""Seeds the stack's own ranking events so that clicks depend on item price (RAT-34 fixture).

The nightly chain trains on `rank_training@v1`, which the featurestore builds from the impression and
click events the stack received. Real traffic carries no learnable signal beyond popularity, so a GBDT
trained on it only ties the fixed-weight baseline. The fixture therefore posts, through the gateway
edge like any browser, impression lists whose clicks depend on item price (cheap items are clicked),
so the model genuinely learns something the baseline does not know. Nothing is forced or faked: the
gate still decides, on events the pipeline reads end to end.

Every seeded event carries the impression id prefix `rat34-` so the data is identifiable in the
event log (an append-only log: it cannot be deleted, only recognised).

    FEATURESTORE_IMAGE   image used to read the latest feature snapshots, default platform-featurestore:local
"""

from __future__ import annotations

import json
import os
import random
import subprocess
import time
import uuid

from tests.e2e.support import tii_support as tii

IMPRESSION_PREFIX = "rat34-"
N_ITEMS = 24
SHOWN = 10
N_LISTS = 500
N_USERS = 40
_BATCH = 60

# Runs inside the featurestore image: listings present in BOTH the latest item_popularity and
# item_attributes snapshots (a seeded impression is joined to the snapshot taken before it).
_PICK = """
import glob, json
import pyarrow.parquet as pq
def latest(view):
    return {r["listing_id"]: r for r in pq.read_table(sorted(glob.glob(f"/features/{view}/v1/*.parquet"))[-1]).to_pylist()}
pop, attrs = latest("item_popularity"), latest("item_attributes")
rows = [(i, int(attrs[i]["price"])) for i in pop if i in attrs and attrs[i]["price"]]
print(json.dumps(rows))
"""


def featurestore_image() -> str:
    return os.getenv("FEATURESTORE_IMAGE", "platform-featurestore:local")


def pick_listings(n: int = N_ITEMS) -> list[tuple[str, int]]:
    """`n` snapshot listings spread evenly over distinct prices, cheapest first."""
    volume = os.getenv("FEATURESTORE_VOLUME", "agora_featurestore_data")
    proc = subprocess.run(
        ["docker", "run", "--rm", "-v", f"{volume}:/features:ro", "--entrypoint", "python",
         featurestore_image(), "-c", _PICK],
        capture_output=True, text=True, timeout=180, check=False,
    )  # fmt: skip
    assert proc.returncode == 0, f"cannot read the feature snapshots:\n{proc.stderr[-2000:]}"
    by_price: dict[int, str] = {}
    for listing, price in sorted(json.loads(proc.stdout.strip().splitlines()[-1])):
        by_price.setdefault(price, listing)
    ordered = sorted((p, i) for p, i in by_price.items())
    assert len(ordered) >= n, f"only {len(ordered)} distinct prices in the feature snapshots"
    step = (len(ordered) - 1) / (n - 1)
    return [(ordered[round(k * step)][1], ordered[round(k * step)][0]) for k in range(n)]


def _beacon(kind: str, listing: str, impression: str, anon: str, position: int) -> dict:
    return {
        "type": kind,
        "listingId": listing,
        "sessionId": f"rat34-sess-{impression}",
        "anonymousId": anon,
        "path": "/e2e/rat34",
        "position": position,
        "placementId": "rat34",
        "impressionId": impression,
        "modelVersion": "rat34",
        "eventId": str(uuid.uuid4()),
    }


def build_beacons(listings: list[tuple[str, int]], seed: int = 34) -> list[dict]:
    """N_LISTS impression lists over the listings; the click probability falls with the price."""
    rng = random.Random(seed)
    run = uuid.uuid4().hex[:8]
    out: list[dict] = []
    for n in range(N_LISTS):
        impression = f"{IMPRESSION_PREFIX}{run}-{n:04d}"
        anon = f"rat34-anon-{run}-{n % N_USERS}"
        for pos, k in enumerate(rng.sample(range(len(listings)), SHOWN), start=1):
            listing = listings[k][0]
            out.append(_beacon("impression", listing, impression, anon, pos))
            if rng.random() < 0.9 * 0.82**k:  # the cheaper the listing, the likelier the click
                out.append(_beacon("click", listing, impression, anon, pos))
    return out


def seed_price_dependent_events() -> int:
    """Post the beacons through the gateway; returns how many were accepted."""
    beacons = build_beacons(pick_listings())
    accepted = 0
    for i in range(0, len(beacons), _BATCH):
        batch = beacons[i : i + _BATCH]
        resp = tii.post_track(batch)
        assert resp.status_code == 202, (resp.status_code, resp.text[:300])
        accepted += int(resp.json().get("accepted", 0))
    assert accepted == len(beacons), f"{accepted} of {len(beacons)} beacons accepted"
    return accepted


# Runs inside the featurestore image: how many exported events carry the fixture's impression prefix.
_COUNT = """
import duckdb
print(duckdb.connect(":memory:").execute(
    "select count(*) from read_parquet('/data/tracking_events.parquet') where impression_id like 'rat34-%'"
).fetchone()[0])
"""


def exported_count() -> int:
    volume = os.getenv("ANALYTICS_VOLUME", "agora_analytics_data")
    proc = subprocess.run(
        ["docker", "run", "--rm", "-v", f"{volume}:/data:ro", "--entrypoint", "python",
         featurestore_image(), "-c", _COUNT],
        capture_output=True, text=True, timeout=180, check=False,
    )  # fmt: skip
    return int(proc.stdout.strip().splitlines()[-1]) if proc.returncode == 0 else 0


def wait_exported(expected: int, timeout_s: float = 480.0) -> None:
    """The analytics worker exports its Parquet every 300 s; the featurestore reads that file."""
    deadline = time.monotonic() + timeout_s
    seen = 0
    while time.monotonic() < deadline:
        seen = exported_count()
        if seen >= expected:
            return
        time.sleep(15)
    raise AssertionError(f"only {seen} of {expected} seeded events reached the Parquet export")


def materialize_now(wrapper: str) -> None:
    """A feature snapshot taken BEFORE the seeded impressions (the trainer joins point in time)."""
    proc = subprocess.run(
        [wrapper, "--profile", "featurestore", "run", "--rm", "featurestore-job"],
        capture_output=True, text=True, timeout=900, check=False,
        env={**os.environ, "AS_OF": ""},
    )  # fmt: skip
    assert proc.returncode == 0, f"featurestore-job failed:\n{(proc.stdout + proc.stderr)[-2000:]}"
