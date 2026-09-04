"""Feature-manifest aggregator + validator (the auto-pipeline entry point).

Discovers every `<repo>/FEATURES.yaml` across the polyrepo, validates each against
`schemas/features.schema.json`, cross-checks `covered_by` pointers against the
scenarios platform-e2e actually contains, and prints a coverage report.

Usage:
    python scripts/features.py            # report + validation
    python scripts/features.py --strict   # exit non-zero on any error (CI gate)

This is the loader step the auto-generator will build on: today it validates and
reports; next it will emit .feature/steps/pages for `status: planned` entries.
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parent.parent
POLYREPO = ROOT.parent
SCHEMA_PATH = ROOT / "schemas" / "features.schema.json"
FEATURES_DIR = ROOT / "tests" / "e2e" / "features"

STATUS_ORDER = ["automated", "planned", "manual", "not-testable"]


def discover_manifests() -> list[Path]:
    return sorted(POLYREPO.glob("*/FEATURES.yaml"))


def load_schema_validator() -> Draft202012Validator:
    import json

    schema = json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))
    return Draft202012Validator(schema)


def scenario_exists(covered_by: str) -> bool:
    """covered_by = '<relpath>.feature::<Scenario name>' -> verify it resolves."""
    if "::" not in covered_by:
        return False
    rel, scenario = covered_by.split("::", 1)
    feature_file = FEATURES_DIR / rel
    if not feature_file.exists():
        return False
    text = feature_file.read_text(encoding="utf-8")
    names = re.findall(r"^\s*Scenario(?: Outline)?:\s*(.+?)\s*$", text, re.MULTILINE)
    return scenario.strip() in {n.strip() for n in names}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--strict", action="store_true", help="exit non-zero on any error")
    args = parser.parse_args()

    validator = load_schema_validator()
    manifests = discover_manifests()
    if not manifests:
        print(f"No FEATURES.yaml found under {POLYREPO}")
        return 0

    errors: list[str] = []
    totals: dict[str, int] = dict.fromkeys(STATUS_ORDER, 0)
    grand_total = 0

    print(f"Feature manifests under {POLYREPO}\n")
    for path in manifests:
        repo = path.parent.name
        try:
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
        except yaml.YAMLError as exc:
            errors.append(f"{repo}: YAML parse error: {exc}")
            continue

        schema_errors = sorted(validator.iter_errors(data), key=lambda e: list(e.path))
        if schema_errors:
            for e in schema_errors:
                loc = "/".join(str(p) for p in e.path) or "<root>"
                errors.append(f"{repo}: schema: {loc}: {e.message}")
            continue

        feats = data.get("features", [])
        by_status: dict[str, int] = dict.fromkeys(STATUS_ORDER, 0)
        for f in feats:
            status = f["status"]
            by_status[status] += 1
            totals[status] += 1
            grand_total += 1
            covered = f.get("covered_by")
            if status == "automated":
                if not covered:
                    errors.append(f"{repo}: '{f['id']}' is automated but has no covered_by")
                elif not scenario_exists(covered):
                    errors.append(f"{repo}: '{f['id']}' covered_by not found: {covered}")
            elif covered:
                errors.append(f"{repo}: '{f['id']}' is {status} but sets covered_by")

        summary = "  ".join(f"{s}={by_status[s]}" for s in STATUS_ORDER if by_status[s])
        print(f"  {repo:<16} {len(feats):>2} features   {summary}")

    autom = totals["automated"]
    coverable = grand_total - totals["not-testable"]
    pct = (autom / coverable * 100) if coverable else 0.0
    print("\nTotals:")
    for s in STATUS_ORDER:
        print(f"  {s:<13} {totals[s]}")
    print(f"  {'coverage':<13} {autom}/{coverable} automated ({pct:.0f}%)")

    if errors:
        print(f"\n{len(errors)} issue(s):")
        for e in errors:
            print(f"  ✗ {e}")
        if args.strict:
            return 1
    else:
        print("\nAll manifests valid; every covered_by resolves. ✓")
    return 0


if __name__ == "__main__":
    sys.exit(main())
