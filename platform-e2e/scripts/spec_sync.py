"""Spec↔e2e coverage check for an OpenSpec change (archive gate).

Given a change id, read its spec-delta `#### Scenario:` titles and check each is
covered by an automated feature in some `<repo>/FEATURES.yaml` (`covered_by` points
to a real `.feature` scenario). Convention: a `.feature` Scenario name should echo
the spec scenario title. Matching is normalized (case/punctuation-insensitive,
substring either direction).

Usage:
    python scripts/spec_sync.py <change-id>            # report
    python scripts/spec_sync.py <change-id> --strict   # exit 1 if any scenario uncovered
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
POLYREPO = ROOT.parent
OPENSPEC = POLYREPO / "openspec"


def _norm(s: str) -> str:
    return re.sub(r"[^a-z0-9]+", " ", s.lower()).strip()


def change_scenarios(change_id: str) -> list[tuple[str, str]]:
    """Return (capability, scenario_title) from a change's spec deltas."""
    base = OPENSPEC / "changes" / change_id / "specs"
    if not base.exists():
        raise FileNotFoundError(f"No spec deltas at {base}")
    out: list[tuple[str, str]] = []
    for spec in base.rglob("spec.md"):
        capability = spec.parent.relative_to(base).as_posix()
        for m in re.finditer(r"^####\s+Scenario:\s*(.+?)\s*$", spec.read_text("utf-8"), re.M):
            out.append((capability, m.group(1)))
    return out


def automated_scenarios() -> list[str]:
    """Every covered_by scenario name across all repo FEATURES.yaml (automated only)."""
    names: list[str] = []
    for manifest in POLYREPO.glob("*/FEATURES.yaml"):
        data = yaml.safe_load(manifest.read_text("utf-8")) or {}
        for feat in data.get("features", []):
            if feat.get("status") == "automated" and feat.get("covered_by"):
                names.append(feat["covered_by"].split("::", 1)[-1])
    return names


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("change_id")
    ap.add_argument("--strict", action="store_true")
    args = ap.parse_args()

    scenarios = change_scenarios(args.change_id)
    if not scenarios:
        print(f"No '#### Scenario:' blocks in change {args.change_id}")
        return 0
    covered = [_norm(n) for n in automated_scenarios()]

    missing = 0
    print(f"Spec↔e2e coverage for change '{args.change_id}':\n")
    for capability, title in scenarios:
        n = _norm(title)
        hit = any(n in c or c in n for c in covered)
        mark = "✓" if hit else "✗"
        if not hit:
            missing += 1
        print(f"  {mark} [{capability}] {title}")

    total = len(scenarios)
    print(f"\n{total - missing}/{total} scenarios have an automated e2e test.")
    if missing:
        print(f"{missing} uncovered — add features/scenarios (skill: spec-to-e2e).")
        if args.strict:
            return 1
    else:
        print("Change is e2e-ready to archive. ✓")
    return 0


if __name__ == "__main__":
    sys.exit(main())
