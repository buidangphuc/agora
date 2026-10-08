"""Random Vietnamese test-data generators (mirrors bds `data.utils.ts` fakerVI)."""

from __future__ import annotations

try:
    from faker import Faker

    _fake = Faker("vi_VN")
except ImportError:
    _fake = None

import random
import uuid


def unique_username(prefix: str = "e2e") -> str:
    """A username that is unique across runs, workers and days.

    The stack keeps every account it was ever seeded with (tens of thousands), and
    register() falls back to login on AlreadyExists, so a colliding name silently
    reuses an old account and its data (a "fresh" seller turned up with a listing).
    The uuid suffix makes a collision practically impossible; the faker part only
    keeps names readable in logs.
    """
    suffix = uuid.uuid4().hex[:12]
    if _fake:
        return f"{prefix}_{_fake.user_name()}_{suffix}".lower()
    return f"{prefix}_user_{suffix}".lower()


def vietnamese_name() -> str:
    if _fake:
        return _fake.name()
    return "Nguyen Van A"


def phone_number() -> str:
    if _fake:
        return _fake.phone_number()
    return f"090{random.randint(1000000, 9999999)}"


def listing_title(brand: str = "Sản phẩm") -> str:
    if _fake:
        return f"{brand} {_fake.word().capitalize()} {_fake.random_number(digits=3)}"
    return f"{brand} Test Item {random.randint(100, 999)}"


def price_vnd(minimum: int = 100_000, maximum: int = 50_000_000) -> int:
    if _fake:
        return _fake.random_int(min=minimum, max=maximum, step=10_000)
    return random.randrange(minimum, maximum, 10_000)
