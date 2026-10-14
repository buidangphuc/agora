"""Static (no browser, no stack) checks: replace the browser-backed `world` fixture so the
autouse `seed_by_tags` hook in tests/conftest.py does not launch Playwright for these tests."""

import pytest


@pytest.fixture
def world():
    return None
