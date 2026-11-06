from __future__ import annotations

import argparse
import os
import subprocess
import sys
import uuid
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from tests.e2e.support import tii_support as tii

QUERY = (
    "select distinct on (price) id from listings where status = 'published' and price > 0 "
    "order by price, created_at desc limit {limit}"
)


def listing_ids(container: str, limit: int) -> list[str]:
    out = subprocess.run(
        [
            "docker",
            "exec",
            container,
            "psql",
            "-U",
            "listing_svc",
            "-d",
            "listing_db",
            "-tAc",
            QUERY.format(limit=limit),
        ],
        capture_output=True,
        text=True,
        check=True,
    )
    return [line.strip() for line in out.stdout.splitlines() if line.strip()]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--listings", type=int, default=40)
    parser.add_argument("--views", type=int, default=3)
    args = parser.parse_args()
    container = os.getenv("POSTGRES_CONTAINER", "agora-postgres-1")
    ids = listing_ids(container, args.listings)
    run = uuid.uuid4().hex[:8]
    beacons = [
        {
            "type": "view",
            "listingId": lid,
            "sessionId": f"envseed-{run}-{i}-{k}",
            "path": f"/listing/{lid}",
        }
        for i, lid in enumerate(ids)
        for k in range(args.views)
    ]
    accepted = 0
    for start in range(0, len(beacons), 60):
        resp = tii.post_track(beacons[start : start + 60])
        if resp.status_code != 202:
            print(f"track refused: {resp.status_code} {resp.text[:300]}", file=sys.stderr)
            return 1
        accepted += int(resp.json().get("accepted", 0))
    print(f"{len(ids)} listings with distinct prices, {accepted} of {len(beacons)} views accepted")
    return 0 if accepted == len(beacons) else 1


if __name__ == "__main__":
    sys.exit(main())
