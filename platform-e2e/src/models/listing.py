"""Domain model: a marketplace listing."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class Listing:
    title: str
    category_id: str = "cat-electronics"
    price: int = 5_000_000
    stock: int = 100
    status: str = "published"
    currency: str = "VND"
    description: str = ""
    listing_id: str | None = None  # populated after create
