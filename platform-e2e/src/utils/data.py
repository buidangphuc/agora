"""Random Vietnamese test-data generators (mirrors bds `data.utils.ts` fakerVI)."""

from __future__ import annotations

try:
    from faker import Faker
    _fake = Faker("vi_VN")
except ImportError:
    _fake = None

import random
import time


def unique_username(prefix: str = "e2e") -> str:
    """A username safe for repeated seeding (register is idempotent-ish)."""
    if _fake:
        return f"{prefix}_{_fake.user_name()}_{_fake.random_number(digits=4)}".lower()
    return f"{prefix}_user_{int(time.time()*1000) % 1000000}".lower()


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
