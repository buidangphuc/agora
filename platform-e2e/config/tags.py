"""Tag -> seed-payload map (mirrors bds `test-tags.ts` / `TAG_PAYLOAD_MAP`).

A scenario tag becomes a pytest marker (pytest-bdd does this automatically). The
autouse `seed_by_tags` fixture in `tests/conftest.py` inspects a scenario's
markers and runs the matching seed before the steps execute.

Account seeding (`needsSeller` / `needsBuyer`) is handled generically by role.
Listing seeding is data-driven here so new heavy scenarios only add a row.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class ListingSeed:
    """Payload for CreateListing used when a scenario needs a listing to exist."""

    title: str = "[E2E] Seeded Listing"
    category_id: str = "cat-electronics"
    price: int = 5_000_000
    stock: int = 100
    status: str = "published"
    description: str = "Sản phẩm seed tự động cho E2E."


# Marker name -> listing payload. Extend with @vipDiamond-style rows as needed.
TAG_PAYLOAD_MAP: dict[str, ListingSeed] = {
    "needsListing": ListingSeed(),
}

# Markers that seed an account of a given role via the gateway API.
ROLE_SEED_MARKERS: dict[str, str] = {
    "needsSeller": "seller",
    "needsBuyer": "buyer",
}
