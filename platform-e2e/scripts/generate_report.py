"""Report helper (mirrors bds generate-report.js).

pytest-html writes a self-contained HTML report via `--html`. This helper just
reports where artifacts landed so CI logs have a stable pointer.
"""

from __future__ import annotations

from pathlib import Path

RESULTS_DIR = Path(__file__).resolve().parent.parent / "test-results"


def main() -> None:
    html = RESULTS_DIR / "report.html"
    junit = RESULTS_DIR / "junit.xml"
    shots = RESULTS_DIR / "screenshots"
    print("E2E report artifacts:")
    print(f"  html   : {html} {'(present)' if html.exists() else '(missing)'}")
    print(f"  junit  : {junit} {'(present)' if junit.exists() else '(missing)'}")
    if shots.exists():
        for png in sorted(shots.glob("*.png")):
            print(f"  failure: {png}")


if __name__ == "__main__":
    main()
