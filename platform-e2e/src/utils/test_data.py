"""Environment-partitioned test data access (mirrors bds `TestDataManager`).

Singleton that reads `test-data/<env>/*.json`, caches parsed files, and exposes
typed lookups. Seeded accounts (`shopee_tech_mall`, `khach_hang_shopee`, ...) come
from `users.json`; their password matches the seed script's `SEED_PASSWORD`.
"""

from __future__ import annotations

import json
from functools import lru_cache
from pathlib import Path

from config.settings import get_settings
from src.models import User

ROOT_DIR = Path(__file__).resolve().parent.parent.parent
TEST_DATA_DIR = ROOT_DIR / "test-data"


class TestDataManager:
    def __init__(self, env: str) -> None:
        self._dir = TEST_DATA_DIR / env
        if not self._dir.exists():
            self._dir = TEST_DATA_DIR / "local"
        self._cache: dict[str, dict] = {}

    def _load(self, filename: str) -> dict:
        if filename not in self._cache:
            path = self._dir / filename
            if not path.exists():
                raise FileNotFoundError(f"Test data file not found: {path}")
            self._cache[filename] = json.loads(path.read_text(encoding="utf-8"))
        return self._cache[filename]

    # ── Users ────────────────────────────────────────────────────────────
    def _users(self) -> list[dict]:
        return self._load("users.json")["users"]

    def get_user_by_role(self, role: str) -> User:
        for u in self._users():
            if u["role"] == role:
                return _to_user(u)
        raise LookupError(f"No seeded user with role={role!r} in {self._dir}")

    def get_user_by_id(self, user_id: str) -> User:
        for u in self._users():
            if u["id"] == user_id:
                return _to_user(u)
        raise LookupError(f"No seeded user with id={user_id!r} in {self._dir}")

    # ── Listings / categories ────────────────────────────────────────────
    def categories(self) -> list[dict]:
        return self._load("listings.json")["categories"]

    def default_search_term(self) -> str:
        return self._load("listings.json")["search_terms"][0]


def _to_user(raw: dict) -> User:
    return User(username=raw["username"], password=raw["password"], role=raw["role"])


@lru_cache(maxsize=1)
def get_test_data_manager() -> TestDataManager:
    return TestDataManager(get_settings().env)
