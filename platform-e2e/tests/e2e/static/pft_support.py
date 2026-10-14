"""Shared helpers for the platform-finetune static tests (git, file and manifest reads only)."""

from __future__ import annotations

import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[4]
FINETUNE = ROOT / "platform-finetune"
GITOPS = ROOT / "platform-gitops"


def run(cmd: list[str], cwd: Path = ROOT, timeout: int = 300) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout
    )  # noqa: S603


def git(*args: str) -> str:
    res = run(["git", *args])
    assert res.returncode == 0, f"git {' '.join(args)} failed: {res.stderr[:300]}"
    return res.stdout
