"""Spec↔e2e coverage check for an OpenSpec change (archive gate).

Given a change id, read its spec-delta `#### Scenario:` titles and check each is
covered by an automated feature in some `<repo>/FEATURES.yaml` (`covered_by` points
to a real `.feature` scenario). Convention: a `.feature` Scenario name should echo
the spec scenario title. Matching is normalized (case/punctuation-insensitive,
substring either direction).

A scenario that cannot be produced end to end is exempt only when BOTH hold: its spec
body has a `**VERIFIED BY**:` line naming the unit test, and a FEATURES.yaml entry with
that title has `status: not-testable`. It is reported as unit-verified, not covered.

Cross-check (tasks.md <-> FEATURES.yaml <-> .feature), reported after the coverage list:
  * e2e-track tasks in the change's tasks.md (line contains `[e2e]` or `track: e2e`) must
    reference what exists via `capability: <cap>` and/or `scenario: <title>` (title in
    backticks or double quotes), resolved against the change's spec deltas.
  * FEATURES.yaml entries tied to the change (title or covered_by matches a spec scenario)
    and not manual/not-testable must have a `.feature` scenario for their acceptance lines;
    `status: automated` with a missing/unresolved scenario is a contradiction.
  * A spec scenario name defined in more than one `.feature` file is an issue only when the
    change's FEATURES entry for it does not pin one file via `covered_by: <file>::<name>`;
    otherwise it is a warning. FEATURES entries are tied to scenarios by normalised equality.
Any cross-check issue fails under --strict; warnings never do.

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
FEATURES_DIR = ROOT / "tests" / "e2e" / "features"


def _norm(s: str) -> str:
    return re.sub(r"[^a-z0-9]+", " ", s.lower()).strip()


def change_scenarios(change_id: str) -> list[tuple[str, str, bool]]:
    """Return (capability, scenario_title, has VERIFIED BY line) from a change's spec deltas."""
    base = OPENSPEC / "changes" / change_id / "specs"
    if not base.exists():
        raise FileNotFoundError(f"No spec deltas at {base}")
    out: list[tuple[str, str, bool]] = []
    for spec in base.rglob("spec.md"):
        capability = spec.parent.relative_to(base).as_posix()
        text = spec.read_text("utf-8")
        heads = list(re.finditer(r"^####\s+Scenario:\s*(.+?)\s*$", text, re.M))
        for i, m in enumerate(heads):
            end = heads[i + 1].start() if i + 1 < len(heads) else len(text)
            nxt = re.search(r"^#{1,3}\s", text[m.end() : end], re.M)
            body = text[m.end() : m.end() + nxt.start()] if nxt else text[m.end() : end]
            out.append((capability, m.group(1), "**VERIFIED BY**:" in body))
    return out


def not_testable_titles() -> set[str]:
    """Normalised titles of FEATURES.yaml entries marked not-testable."""
    titles: set[str] = set()
    for manifest in POLYREPO.glob("*/FEATURES.yaml"):
        data = yaml.safe_load(manifest.read_text("utf-8")) or {}
        for feat in data.get("features", []):
            if feat.get("status") == "not-testable" and feat.get("title"):
                titles.add(_norm(feat["title"]))
    return titles


def automated_scenarios() -> list[str]:
    """Every covered_by scenario name across all repo FEATURES.yaml (automated only)."""
    names: list[str] = []
    for manifest in POLYREPO.glob("*/FEATURES.yaml"):
        data = yaml.safe_load(manifest.read_text("utf-8")) or {}
        for feat in data.get("features", []):
            if feat.get("status") == "automated" and feat.get("covered_by"):
                names.append(feat["covered_by"].split("::", 1)[-1])
    return names


_SCEN_RE = re.compile(r"^\s*Scenario(?: Outline)?:\s*(.+?)\s*$", re.M)


def feature_scenarios() -> dict[str, list[str]]:
    """Scenario name -> list of .feature files (relative) defining it, one entry per definition."""
    out: dict[str, list[str]] = {}
    if not FEATURES_DIR.exists():
        return out
    for f in sorted(FEATURES_DIR.rglob("*.feature")):
        rel = f.relative_to(FEATURES_DIR).as_posix()
        for name in _SCEN_RE.findall(f.read_text("utf-8")):
            out.setdefault(name.strip(), []).append(rel)
    return out


def _scenario_defined(covered_by: str, defined: dict[str, list[str]]) -> bool:
    if "::" not in covered_by:
        return False
    rel, name = covered_by.split("::", 1)
    return rel in defined.get(name.strip(), [])


def e2e_task_refs(change_id: str) -> list[tuple[str, str | None, str | None]]:
    """(task line, capability ref, scenario ref) for each e2e-track task in tasks.md."""
    path = OPENSPEC / "changes" / change_id / "tasks.md"
    if not path.exists():
        return []
    out = []
    for line in path.read_text("utf-8").splitlines():
        if not re.match(r"\s*-\s*\[[ xX]\]", line):
            continue
        if not re.search(r"\[e2e\]|track:\s*e2e", line, re.I):
            continue
        cap = re.search(r"capability:\s*`?([A-Za-z0-9._/-]+)`?", line)
        scn = re.search(r"scenario:\s*(?:`([^`]+)`|\"([^\"]+)\")", line)
        out.append(
            (
                line.strip(),
                cap.group(1) if cap else None,
                (scn.group(1) or scn.group(2)) if scn else None,
            )
        )
    return out


def cross_check(
    change_id: str, scenarios: list[tuple[str, str, bool]]
) -> tuple[list[str], list[str]]:
    """tasks.md <-> FEATURES.yaml <-> .feature consistency (issues, warnings) for a change."""
    issues: list[str] = []
    warnings: list[str] = []
    tied_resolved: set[str] = set()  # normalised scenario titles with an unambiguous covered_by
    caps = {c for c, _, _ in scenarios}
    titles = {(c, _norm(t)) for c, t, _ in scenarios}
    defined = feature_scenarios()

    # 1. e2e-track tasks reference things that exist
    for line, cap, scn in e2e_task_refs(change_id):
        if not cap and not scn:
            issues.append(f"tasks.md: e2e task has no capability:/scenario: reference: {line}")
        if cap and cap not in caps:
            issues.append(
                f"tasks.md: e2e task references unknown capability '{cap}' (spec has: {sorted(caps)})"
            )
        if scn:
            hit = any(n == _norm(scn) and (not cap or c == cap) for c, n in titles)
            if not hit:
                issues.append(f"tasks.md: e2e task references unknown scenario '{scn}'")

    # 2. FEATURES.yaml entries tied to this change
    norms = [n for _, n in titles]
    for manifest in sorted(POLYREPO.glob("*/FEATURES.yaml")):
        repo = manifest.parent.name
        data = yaml.safe_load(manifest.read_text("utf-8")) or {}
        for feat in data.get("features", []):
            cov = feat.get("covered_by") or ""
            keys = [_norm(feat.get("title", "")), _norm(cov.split("::", 1)[-1]) if cov else ""]
            tied = [k for k in keys if k and k in norms]
            if not tied:
                continue
            status, fid = feat.get("status"), feat.get("id")
            resolved = bool(cov) and _scenario_defined(cov, defined)
            if resolved:
                tied_resolved.update(tied)
            if status == "automated" and not resolved:
                issues.append(
                    f"{repo}: '{fid}' is automated but no matching .feature scenario ({cov or 'no covered_by'})"
                )
            elif status == "planned" and feat.get("acceptance") and not resolved:
                issues.append(f"{repo}: '{fid}' has acceptance lines but no .feature scenario yet")

    # 3. duplicate scenario names. An error only when this change's FEATURES entry does not
    #    pin the scenario to one file (covered_by rel::name), so the name resolves ambiguously;
    #    otherwise the duplicate is just a warning (generic names like "Empty tab" are common).
    for _, title, _ in scenarios:
        n = _norm(title)
        for name, files in defined.items():
            if _norm(name) == n and len(files) > 1:
                msg = f"duplicate scenario '{name}' defined {len(files)}x: {sorted(set(files))}"
                if n in tied_resolved:
                    warnings.append(msg + " (covered_by pins one file)")
                else:
                    tied = any(
                        _norm(f.get("title", "")) == n
                        for m in POLYREPO.glob("*/FEATURES.yaml")
                        for f in (yaml.safe_load(m.read_text("utf-8")) or {}).get("features", [])
                    )
                    (issues if tied else warnings).append(msg)
    return sorted(set(issues)), sorted(set(warnings))


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
    exempt = not_testable_titles()

    missing = unit = 0
    print(f"Spec↔e2e coverage for change '{args.change_id}':\n")
    for capability, title, verified_by in scenarios:
        n = _norm(title)
        hit = any(n in c or c in n for c in covered)
        if hit:
            mark = "✓"
        elif verified_by and n in exempt:
            mark = "u"
            unit += 1
        else:
            mark = "✗"
            missing += 1
        print(f"  {mark} [{capability}] {title}")

    issues, warnings = cross_check(args.change_id, scenarios)
    total = len(scenarios)
    print(f"\n{total - missing - unit}/{total} scenarios have an automated e2e test.")
    if unit:
        print(f"{unit} unit-verified (VERIFIED BY + not-testable), exempt from e2e.")
    if missing:
        print(f"{missing} uncovered — add features/scenarios (skill: spec-to-e2e).")
    else:
        print("Change is e2e-ready to archive. ✓")
    if issues:
        print(f"\n{len(issues)} cross-check issue(s) (tasks.md <-> FEATURES.yaml <-> .feature):")
        for i in issues:
            print(f"  ✗ {i}")
    if warnings:
        print(f"\n{len(warnings)} warning(s):")
        for w in warnings:
            print(f"  ! {w}")
    if args.strict and (missing or issues):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
