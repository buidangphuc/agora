#!/usr/bin/env python3
"""Fail if any manifest enables team-identity's admin seed or hardcodes its password.

Rule (OpenSpec secure-seller-analytics-and-admin-seed, admin-bootstrap): deployment
manifests SHALL NOT set SEED_ADMIN_ENABLED to a truthy value and SHALL NOT contain a
SEED_ADMIN_PASSWORD literal. A first admin is created out of band, or by enabling the
seed once with a secret that never lands in git.

Usage: check_identity_seed.py [ROOT]   (default: the platform-gitops dir next to scripts/)
Exit 0 = clean, 1 = violations (printed as path:line: message). Stdlib only.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

SKIP_DIRS = {".git", ".git__snapshot_disabled", "node_modules", ".claude"}
SUFFIXES = {".yaml", ".yml", ".tpl", ".json"}
TRUTHY = r"(?:true|yes|on|1)"
# "KEY: value", "KEY=value" or `- KEY=value`; value optionally quoted.
ENABLED_INLINE = re.compile(
    rf"SEED_ADMIN_ENABLED[\"']?\s*[:=]\s*[\"']?{TRUTHY}[\"']?\s*(?:#.*)?$", re.I
)
PASSWORD_INLINE = re.compile(r"SEED_ADMIN_PASSWORD[\"']?\s*[:=]\s*(.*)$")
# `name: SEED_ADMIN_X` followed (within 3 lines) by `value: ...`.
NAME_LINE = re.compile(r"name:\s*[\"']?(SEED_ADMIN_(?:ENABLED|PASSWORD))[\"']?\s*$")
VALUE_LINE = re.compile(r"value:\s*(.*)$")


def _scalar(raw: str) -> str:
    raw = raw.split(" #")[0].strip().strip("\"'").strip()
    return raw


def _is_reference(value: str) -> bool:
    """A templated/secret reference rather than a literal secret."""
    return value.startswith(("{{", "${", "$(")) or value == ""


def scan_text(text: str, path: str) -> list[str]:
    out: list[str] = []
    lines = text.splitlines()
    for i, line in enumerate(lines):
        body = line.strip()
        if body.startswith("#"):
            continue
        if ENABLED_INLINE.search(body):
            out.append(f"{path}:{i + 1}: SEED_ADMIN_ENABLED must not be enabled in manifests")
        m = PASSWORD_INLINE.search(body)
        if m and not _is_reference(_scalar(m.group(1))):
            out.append(f"{path}:{i + 1}: SEED_ADMIN_PASSWORD must not be a literal in manifests")
        n = NAME_LINE.search(body)
        if n:
            for j in range(i + 1, min(i + 4, len(lines))):
                v = VALUE_LINE.search(lines[j].strip())
                if not v:
                    continue
                val = _scalar(v.group(1))
                if n.group(1) == "SEED_ADMIN_ENABLED" and re.fullmatch(TRUTHY, val, re.I):
                    out.append(
                        f"{path}:{j + 1}: SEED_ADMIN_ENABLED must not be enabled in manifests"
                    )
                if n.group(1) == "SEED_ADMIN_PASSWORD" and not _is_reference(val):
                    out.append(
                        f"{path}:{j + 1}: SEED_ADMIN_PASSWORD must not be a literal in manifests"
                    )
                break
    return out


def scan(root: Path) -> list[str]:
    problems: list[str] = []
    for p in sorted(root.rglob("*")):
        if not p.is_file() or p.suffix not in SUFFIXES:
            continue
        if SKIP_DIRS & set(p.relative_to(root).parts):
            continue
        problems += scan_text(p.read_text(encoding="utf-8", errors="replace"), str(p.relative_to(root)))
    return problems


def main(argv: list[str]) -> int:
    root = Path(argv[1]) if len(argv) > 1 else Path(__file__).resolve().parent.parent
    problems = scan(root)
    for p in problems:
        print(p)
    if problems:
        print(f"FAIL: {len(problems)} admin-seed violation(s) under {root}")
        return 1
    print(f"OK: no admin seed enabled or password literal under {root}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
