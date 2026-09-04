"""Datadog upload stub (mirrors bds scripts/upload-test-results.js).

Parses the JUnit XML pytest emits and would push per-test logs + gauge metrics to
Datadog. Left as a stub: it prints a summary and no-ops unless DD_API_KEY is set.
Wire the real HTTP calls (api/v1/series, api/v2/logs) when metrics are needed.
"""

from __future__ import annotations

import os
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

RESULTS = Path(__file__).resolve().parent.parent / "test-results" / "junit.xml"


def summarize(path: Path) -> dict[str, int]:
    if not path.exists():
        print(f"[upload] no results at {path}; run pytest with --junitxml first")
        return {}
    root = ET.parse(path).getroot()
    suite = root if root.tag == "testsuite" else root.find("testsuite")
    if suite is None:
        return {}
    return {
        "tests": int(suite.get("tests", 0)),
        "failures": int(suite.get("failures", 0)),
        "errors": int(suite.get("errors", 0)),
        "skipped": int(suite.get("skipped", 0)),
    }


def main() -> int:
    stats = summarize(RESULTS)
    if not stats:
        return 0
    print(f"[upload] {stats}")
    if not (os.getenv("DD_API_KEY") or os.getenv("DATADOG_API_KEY")):
        print("[upload] DD_API_KEY not set — skipping Datadog push (stub).")
        return 0
    print("[upload] TODO: POST x_qa.test_runs.* gauges to Datadog /api/v1/series")
    return 0


if __name__ == "__main__":
    sys.exit(main())
