#!/usr/bin/env python3
"""Validate a parallel-scrum plan/ directory before fan-out.

Checks:
  1. 00-spec.md exists, has '## Locked' line, has zero [UNRESOLVED tags
  2. Task packets parse (wave number, write-set section present, verify command present)
  3. Write-sets are disjoint WITHIN each wave (the core invariant)
  4. No packet writes a file that another same-wave packet reads-only via glob overlap warning

Also supports post-implementation audit:
  --audit <task-packet> : compare `git diff --name-only <base>` against the packet's
  write-set; exits non-zero if the agent touched files outside its declared ownership.

Usage:
  python validate_plan.py plan/
  python validate_plan.py plan/ --audit plan/wave1-task2-geo.md --base main
"""
import argparse
import re
import subprocess
import sys
from fnmatch import fnmatch
from pathlib import Path

PACKET_RE = re.compile(r"wave(\d+)-task(\d+)-(.+)\.md$")


def parse_packet(path: Path):
    m = PACKET_RE.search(path.name)
    if not m:
        return None
    text = path.read_text(encoding="utf-8")
    wave = int(m.group(1))

    def section(header_prefix):
        pat = re.compile(rf"^##\s*{header_prefix}.*?$(.*?)(?=^##\s|\Z)",
                         re.M | re.S | re.I)
        sm = pat.search(text)
        return sm.group(1) if sm else None

    ws_body = section(r"Write-set")
    writes = []
    if ws_body:
        for line in ws_body.splitlines():
            line = line.strip()
            if line.startswith("-"):
                entry = line.lstrip("- ").split("(")[0].strip().strip("`")
                if entry:
                    writes.append(entry)
    has_verify = section(r"Verify") is not None
    return {"path": path, "wave": wave, "slug": m.group(3),
            "writes": writes, "has_verify": has_verify}


def overlap(a: str, b: str) -> bool:
    """True if two write-set entries can claim the same file (exact, prefix-dir, or glob)."""
    if a == b:
        return True
    for x, y in ((a, b), (b, a)):
        if y.startswith(x.rstrip("/") + "/"):  # x is a parent dir of y
            return True
        if "*" in x and (fnmatch(y, x) or y.startswith(x.split("*")[0])):
            return True
    return False


def validate(plan_dir: Path) -> int:
    errors, warnings = [], []

    spec = plan_dir / "00-spec.md"
    if not spec.exists():
        errors.append("00-spec.md missing")
    else:
        stext = spec.read_text(encoding="utf-8")
        if "## Locked" not in stext:
            errors.append("00-spec.md has no '## Locked' line — spec not confirmed (Step 2.5)")
        n_unres = stext.count("[UNRESOLVED")
        if n_unres:
            errors.append(f"00-spec.md has {n_unres} [UNRESOLVED] tag(s)")
        if len(stext.splitlines()) > 400:
            warnings.append(f"00-spec.md is {len(stext.splitlines())} lines (>400) — trim it, "
                            "it goes into every agent's context")

    packets = [p for p in (parse_packet(f) for f in sorted(plan_dir.glob("wave*-task*.md"))) if p]
    if not packets:
        errors.append("no task packets found (wave{N}-task{M}-{slug}.md)")

    for p in packets:
        if not p["writes"]:
            errors.append(f"{p['path'].name}: empty or missing '## Write-set' section")
        if not p["has_verify"]:
            errors.append(f"{p['path'].name}: missing '## Verify' section")

    # core invariant: disjoint write-sets within a wave
    by_wave = {}
    for p in packets:
        by_wave.setdefault(p["wave"], []).append(p)
    for wave, ps in sorted(by_wave.items()):
        if wave == 0 and len(ps) > 1:
            warnings.append(f"wave 0 has {len(ps)} tasks — foundation wave should usually be solo")
        for i in range(len(ps)):
            for j in range(i + 1, len(ps)):
                for wa in ps[i]["writes"]:
                    for wb in ps[j]["writes"]:
                        if overlap(wa, wb):
                            errors.append(
                                f"wave {wave} CONFLICT: '{wa}' ({ps[i]['slug']}) vs "
                                f"'{wb}' ({ps[j]['slug']}) — write-sets must be disjoint")
        if len(ps) > 5:
            warnings.append(f"wave {wave} has {len(ps)} parallel tasks (>5) — consider merging")

    for w in warnings:
        print(f"⚠️  {w}")
    for e in errors:
        print(f"❌ {e}")
    if not errors:
        print(f"✅ plan valid: {len(packets)} packets, {len(by_wave)} waves")
    return 1 if errors else 0


def audit(packet_path: Path, base: str) -> int:
    p = parse_packet(packet_path)
    if not p:
        print(f"❌ cannot parse packet name: {packet_path.name}")
        return 1
    out = subprocess.run(["git", "diff", "--name-only", base],
                         capture_output=True, text=True, check=True).stdout
    changed = [l.strip() for l in out.splitlines() if l.strip()]
    violations = [f for f in changed
                  if not any(overlap(f, w) for w in p["writes"])]
    if violations:
        print(f"❌ {packet_path.name}: agent wrote OUTSIDE its write-set:")
        for v in violations:
            print(f"   - {v}")
        return 1
    print(f"✅ audit clean: {len(changed)} changed file(s), all within write-set")
    return 0


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("plan_dir", type=Path)
    ap.add_argument("--audit", type=Path, help="task packet to audit against git diff")
    ap.add_argument("--base", default="main", help="git base ref for --audit")
    args = ap.parse_args()
    if args.audit:
        sys.exit(audit(args.audit, args.base))
    sys.exit(validate(args.plan_dir))
