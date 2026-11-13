"""Seed the recs-home-feed-ranker Flipt flag (disabled, idempotent).

    python -m scripts.seed_flipt_flags --url http://localhost:8080

An existing flag is never modified and no flag is ever enabled.
"""

from __future__ import annotations

import argparse
import json

import httpx

FLAG_KEY = "recs-home-feed-ranker"
FLAGS_PATH = "/api/v1/namespaces/default/flags"
SIMILARITY_ATTACHMENT = json.dumps({"ranking": {"model": "cosine_rank"}})


def build_client(url: str) -> httpx.Client:
    return httpx.Client(base_url=url, timeout=10.0)


def seed_flags(client: httpx.Client) -> bool:
    existing = client.get(f"{FLAGS_PATH}/{FLAG_KEY}")
    if existing.status_code == 200:
        return False
    if existing.status_code != 404:
        existing.raise_for_status()
    client.post(
        FLAGS_PATH,
        json={
            "key": FLAG_KEY,
            "name": FLAG_KEY,
            "type": "VARIANT_FLAG_TYPE",
            "enabled": False,
        },
    ).raise_for_status()
    for variant in (
        {"key": "control", "name": "control"},
        {
            "key": "similarity",
            "name": "similarity",
            "attachment": SIMILARITY_ATTACHMENT,
        },
    ):
        client.post(
            f"{FLAGS_PATH}/{FLAG_KEY}/variants", json=variant
        ).raise_for_status()
    return True


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=(__doc__ or "").split("\n\n")[0])
    parser.add_argument("--url", default="http://localhost:8080")
    args = parser.parse_args(argv)
    with build_client(args.url) as client:
        created = seed_flags(client)
    print(f"{FLAG_KEY}: {'created' if created else 'exists, untouched'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
