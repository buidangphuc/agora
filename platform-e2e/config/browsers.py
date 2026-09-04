"""Chromium launch options tuned for CI-stable headless runs.

Mirrors bds `browsers.ts`. Kept minimal — one browser (Chromium); add firefox /
webkit here if the suite grows to need them.
"""

from __future__ import annotations

CHROMIUM_ARGS: list[str] = [
    "--no-sandbox",
    "--headless=new",
    "--disable-dev-shm-usage",
    "--disable-software-rasterizer",
    "--disable-gpu",
    "--disable-background-timer-throttling",
    "--disable-backgrounding-occluded-windows",
    "--disable-renderer-backgrounding",
]


def launch_options(*, headless: bool) -> dict:
    return {"headless": headless, "args": CHROMIUM_ARGS}
