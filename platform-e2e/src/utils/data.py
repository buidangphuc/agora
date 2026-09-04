"""Random Vietnamese test-data generators (mirrors bds `data.utils.ts` fakerVI)."""

from __future__ import annotations

from faker import Faker

_fake = Faker("vi_VN")


def unique_username(prefix: str = "e2e") -> str:
    """A username safe for repeated seeding (register is idempotent-ish)."""
    return f"{prefix}_{_fake.user_name()}_{_fake.random_number(digits=4)}".lower()


def vietnamese_name() -> str:
    return _fake.name()


def phone_number() -> str:
    return _fake.phone_number()


def listing_title(brand: str = "Sản phẩm") -> str:
    return f"{brand} {_fake.word().capitalize()} {_fake.random_number(digits=3)}"


def price_vnd(minimum: int = 100_000, maximum: int = 50_000_000) -> int:
    return _fake.random_int(min=minimum, max=maximum, step=10_000)
