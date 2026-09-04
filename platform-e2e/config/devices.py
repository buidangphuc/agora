"""Device emulation descriptors for WAP scenarios.

The `DEVICE` env var (e.g. `DEVICE='iPhone 12'`) selects a Playwright built-in
device descriptor at context-creation time. Desktop is the default when unset.
"""

from __future__ import annotations

DESKTOP_VIEWPORT = {"width": 1280, "height": 720}

# Names must match Playwright's built-in device registry (playwright.devices).
SUPPORTED_DEVICES = {"iPhone 12", "Pixel 7", "iPhone 13", "Galaxy S9+"}


def is_mobile_device(device: str | None) -> bool:
    return bool(device) and device in SUPPORTED_DEVICES
