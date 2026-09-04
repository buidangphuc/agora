"""Named timeouts (ms). Prefer these over magic numbers; never use fixed sleeps."""

from __future__ import annotations

SHORT = 5_000
DEFAULT = 10_000
NAVIGATION = 30_000
LONG = 60_000
