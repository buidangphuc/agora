#!/usr/bin/env python3
"""ASDLC gate W1-5: forbid mocking the system's OWN services in e2e / integration tests.

Usage: no_self_mock.py [paths...] [--base REF]
Output: path:line rule message   (exit 1 when any finding)
Allowlist (external deps only): `# asdlc: allow-mock <reason>` / `// asdlc: allow-mock <reason>`
on the offending line or within the 3 lines above it.
Without paths: every in-scope tracked file (or only those changed since --base).
Explicit paths are scanned regardless of scope.
"""
from __future__ import annotations

import argparse
import ast
import re
import subprocess
import sys
from pathlib import Path

ALLOW = re.compile(r"asdlc:\s*allow-mock\b")
OWN_PY_ROOTS = ("app", "team_ai", "internal", "src.api")
SKIP_PARTS = {".venv", "node_modules", "__pycache__", ".git"}
FIXTURE_PREFIX = "scripts/asdlc/tests/fixtures/"


def repo_root() -> Path:
    out = subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True)
    return Path(out.stdout.strip()) if out.returncode == 0 else Path.cwd()


def in_scope(rel: str) -> bool:
    if rel.startswith(FIXTURE_PREFIX):
        return False
    name = rel.rsplit("/", 1)[-1]
    if rel.startswith("platform-e2e/") and name.endswith((".py", ".ts")):
        return True
    if name.endswith("_integration_test.go") or ("/integration/" in "/" + rel and rel.endswith(".go")):
        return True
    if rel.startswith("team-ai/tests/integration/") and name.endswith(".py"):
        return True
    if "/integration/" in "/" + rel and name.endswith((".py", ".ts")):
        return True
    return False


def allowed(lines: list[str], idx: int) -> bool:
    return any(ALLOW.search(lines[i]) for i in range(max(0, idx - 3), idx + 1))


# ---------------- Go ----------------
GO_GOMOCK = re.compile(r'"(github\.com/golang/mock|go\.uber\.org/mock)[^"]*"|\bgomock\.|\bmockgen\b')
GO_CALLOPT = re.compile(r"^func\s*\(\s*\w*\s*\*?\w+\s*\)\s*\w+\(.*\.\.\.\s*grpc\.CallOption")
GO_EMBED_CLIENT = re.compile(r"^\s+\*?(?:\w+\.)\w+Client\s*$")
GO_FAKE_TYPE = re.compile(r"^type\s+((?:[Ff]ake|[Mm]ock|[Ss]tub)\w*)\s+struct\b")
GO_OWN_IMPORT = re.compile(r'"[^"]*(platform-core/packages/proto|/internal/)[^"]*"')


def scan_go(rel: str, lines: list[str]) -> list[tuple[int, str, str]]:
    res = []
    own_import = any(GO_OWN_IMPORT.search(l) for l in lines if l.lstrip().startswith(('"', 'import', '_ "')) or re.match(r'\s*\w*\s*"', l))
    in_struct = None
    for i, l in enumerate(lines):
        if allowed(lines, i):
            in_struct = None if not l.startswith("}") else in_struct
            continue
        if GO_GOMOCK.search(l):
            res.append((i + 1, "go-gomock", "gomock/mockgen used; integration/e2e must hit the real service"))
        elif GO_CALLOPT.match(l):
            res.append((i + 1, "go-fake-grpc-client", "fake implementing a generated gRPC client interface"))
        m = GO_FAKE_TYPE.match(l)
        if m:
            in_struct = m.group(1)
            if own_import:
                res.append((i + 1, "go-fake-own-type", f"fake/mock/stub type {m.group(1)} in a file importing own internal/proto packages"))
        elif in_struct:
            if l.startswith("}"):
                in_struct = None
            elif GO_EMBED_CLIENT.match(l):
                res.append((i + 1, "go-fake-grpc-client", f"{in_struct} embeds a generated gRPC client interface"))
    return res


# ---------------- Python ----------------
def _own(name: str) -> bool:
    return any(name == r or name.startswith(r + ".") for r in OWN_PY_ROOTS)


def _dotted(n: ast.AST) -> str:
    parts = []
    while isinstance(n, ast.Attribute):
        parts.append(n.attr)
        n = n.value
    if isinstance(n, ast.Name):
        parts.append(n.id)
        return ".".join(reversed(parts))
    return ""


def scan_py(rel: str, lines: list[str]) -> list[tuple[int, str, str]]:
    try:
        tree = ast.parse("\n".join(lines))
    except SyntaxError:
        return []
    own_names: set[str] = set()
    for n in ast.walk(tree):
        if isinstance(n, ast.ImportFrom) and n.module and _own(n.module):
            own_names.update(a.asname or a.name for a in n.names)
        elif isinstance(n, ast.Import):
            for a in n.names:
                if _own(a.name):
                    own_names.add(a.asname or a.name.split(".")[0])
    res = []
    for n in ast.walk(tree):
        if not isinstance(n, ast.Call):
            continue
        fn = _dotted(n.func)
        last = fn.rsplit(".", 1)[-1]
        if not (last in ("patch", "setattr", "setitem", "object") and ("patch" in fn or "monkeypatch" in fn or "mocker" in fn)):
            continue
        if not n.args:
            continue
        a0 = n.args[0]
        hit = None
        if isinstance(a0, ast.Constant) and isinstance(a0.value, str):
            if _own(a0.value):
                hit = a0.value
        else:
            root = _dotted(a0).split(".")[0]
            if root and root in own_names:
                hit = _dotted(a0)
        if hit and not allowed(lines, n.lineno - 1):
            res.append((n.lineno, "py-mock-own-package", f"patching own code ({hit}); use the real service or allowlist an external dependency"))
    return res


# ---------------- TS (Playwright) ----------------
TS_ROUTE = re.compile(r"\.route\(")
GATEWAY = re.compile(r"/api\b|:8080|\bapi/")
MOCKING = re.compile(r"\.fulfill\(|\.abort\(|fulfill:|abort:")


def scan_ts(rel: str, lines: list[str]) -> list[tuple[int, str, str]]:
    res = []
    for i, l in enumerate(lines):
        if not TS_ROUTE.search(l) or allowed(lines, i):
            continue
        window = "\n".join(lines[i : i + 12])
        head = "\n".join(lines[i : i + 3])
        if GATEWAY.search(head) and MOCKING.search(window):
            res.append((i + 1, "ts-route-own-gateway", "page.route stubs a response of our gateway /api (:8080)"))
    return res


# page.route in python Playwright (platform-e2e step definitions)
PY_ROUTE = re.compile(r"\.route\(")


def scan_py_route(rel: str, lines: list[str]) -> list[tuple[int, str, str]]:
    res = []
    for i, l in enumerate(lines):
        if not PY_ROUTE.search(l) or allowed(lines, i):
            continue
        head = "\n".join(lines[i : i + 4])
        window = "\n".join(lines[max(0, i - 12) : i + 12])
        if GATEWAY.search(head) and MOCKING.search(window):
            res.append((i + 1, "py-route-own-gateway", "page.route stubs a response of our gateway /api (:8080)"))
    return res


def scan_file(path: Path, rel: str) -> list[tuple[int, str, str]]:
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return []
    if rel.endswith(".go"):
        return scan_go(rel, lines)
    if rel.endswith(".py"):
        return sorted(scan_py(rel, lines) + scan_py_route(rel, lines))
    if rel.endswith((".ts", ".tsx")):
        return scan_ts(rel, lines)
    return []


def git(root: Path, *args: str) -> list[str]:
    out = subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True)
    return [l for l in out.stdout.splitlines() if l]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("paths", nargs="*")
    ap.add_argument("--base")
    a = ap.parse_args()
    root = repo_root()
    files: list[tuple[Path, str]] = []
    if a.paths:
        for p in a.paths:
            pp = Path(p).resolve()
            cands = [f for f in pp.rglob("*") if f.is_file()] if pp.is_dir() else [pp]
            for f in cands:
                if SKIP_PARTS & set(f.parts):
                    continue
                try:
                    rel = f.relative_to(root).as_posix()
                except ValueError:
                    rel = f.as_posix()
                files.append((f, rel))
    else:
        if a.base:
            rels = git(root, "diff", "--name-only", "--diff-filter=ACMR", f"{a.base}...HEAD")
            rels += git(root, "diff", "--name-only", "--diff-filter=ACMR", "HEAD")
            rels = sorted(set(rels))
        else:
            rels = git(root, "ls-files")
        files = [(root / r, r) for r in rels if in_scope(r) and (root / r).is_file()]
    n = 0
    for f, rel in files:
        for line, rule, msg in scan_file(f, rel):
            print(f"{rel}:{line} {rule} {msg}")
            n += 1
    print(f"no_self_mock: {n} finding(s) in {len(files)} file(s)", file=sys.stderr)
    return 1 if n else 0


if __name__ == "__main__":
    sys.exit(main())
