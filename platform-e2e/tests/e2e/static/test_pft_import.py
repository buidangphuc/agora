"""Static checks for the platform-finetune subtree import (add-platform-finetune, spec requirement
"The finetune pipeline is imported with history as platform-finetune").

No stack: git plumbing, file reads and detect-secrets only.
"""

from __future__ import annotations

import json
import shutil
import tempfile
from pathlib import Path

import pytest

from tests.e2e.static.pft_support import FINETUNE, GITOPS, ROOT, git, run

NOT_PORTED = [
    "src/datascience/serve/local.py",
    "src/datascience/package.py",
    "src/datascience/optimize.py",
    "src/datascience/serve_analyzer.py",
    "src/datascience/serve/triton.py",
    "src/datascience/serve/deploy_sagemaker.py",
]
# Tokens by which a compose service or a gitops manifest could reference a not-ported file.
NOT_PORTED_TOKENS = [
    "serve/local",
    "serve.local",
    "datascience/package",
    "datascience.package",
    "datascience/optimize",
    "datascience.optimize",
    "serve_analyzer",
    "serve/triton",
    "serve.triton",
    "deploy_sagemaker",
]


def _subtree_split_parent() -> str:
    """The imported MLOps history tip: second parent of the `git subtree add` commit."""
    out = git(
        "log", "--format=%H", "--grep=^git-subtree-dir: platform-finetune$", "-n", "1"
    ).strip()
    assert out, "no `git subtree add --prefix=platform-finetune` commit found in history"
    parents = git("rev-list", "--parents", "-n", "1", out).split()
    assert len(parents) == 3, f"subtree add commit {out[:8]} is not a merge of the MLOps history"
    return parents[2]


# --- scenario: History is kept and the local stack is dropped ----------------------------------


def test_history_kept_and_local_stack_dropped():
    # `git log -- platform-finetune` lists only the merge for a subtree add (the MLOps commits sit
    # at the repo root on the second parent), so count the imported history by ancestry instead.
    imported = int(git("rev-list", "--count", _subtree_split_parent()).strip())
    assert imported > 1, f"expected more than one MLOps commit, found {imported}"
    assert not (
        FINETUNE / "ops" / "docker-compose.yml"
    ).exists(), "platform-finetune/ops/docker-compose.yml must be deleted (local stack dropped)"
    assert git("ls-files", "platform-finetune/ops/docker-compose.yml").strip() == ""


# --- scenario: Broken serving files are marked and not deployed --------------------------------


def test_serving_files_marked_not_ported():
    unmarked = []
    for rel in NOT_PORTED:
        path = FINETUNE / rel
        assert path.exists(), f"{rel} missing from the import"
        head = "\n".join(path.read_text("utf-8").splitlines()[:8])
        if "NOT PORTED" not in head:
            unmarked.append(rel)
    assert not unmarked, f"first lines lack a NOT PORTED notice: {unmarked}"

    scanned = [ROOT / "docker-compose.services.yaml", *sorted(GITOPS.rglob("*.yaml"))]
    hits = []
    for f in scanned:
        text = f.read_text("utf-8", errors="ignore")
        if f.name == "docker-compose.services.yaml" or "finetune" in text or "finetune" in str(f):
            hits += [f"{f.relative_to(ROOT)}: {t}" for t in NOT_PORTED_TOKENS if t in text]
    assert not hits, f"compose/gitops reference not-ported files: {hits}"
    assert (GITOPS / "platform" / "finetune").is_dir(), (
        "platform-gitops/platform/finetune does not exist yet, so 'no manifest references' "
        "cannot be shown on the real manifest"
    )


# --- scenario: Artifacts are not committed -----------------------------------------------------


def test_artifacts_untracked():
    assert git("ls-files", "platform-finetune/artifacts").strip() == ""
    ignored = run(["git", "check-ignore", "-q", "platform-finetune/artifacts/registry/x"])
    assert ignored.returncode == 0, "platform-finetune/artifacts/ is not gitignored"


# --- scenario: The imported history has no secrets ---------------------------------------------


def _detect_secrets() -> list[str]:
    if shutil.which("detect-secrets"):
        return ["detect-secrets"]
    if shutil.which("uvx"):
        return ["uvx", "detect-secrets"]
    pytest.fail("detect-secrets is not installed (pip install detect-secrets, or have uvx)")


def _findings(args: list[str]) -> dict:
    res = run([*_detect_secrets(), "scan", "--all-files", *args], timeout=900)
    assert res.returncode == 0, f"detect-secrets failed: {res.stderr[:400]}"
    return json.loads(res.stdout)["results"]


def _summary(found: dict) -> dict:
    return {k: sorted({x["type"] for x in v}) for k, v in found.items()}


def test_detect_secrets_zero_findings():
    files = [f for f in git("ls-files", "platform-finetune").splitlines() if f]
    assert files, "platform-finetune has no tracked files"
    tree = _findings(files)
    with tempfile.TemporaryDirectory() as tmp:
        # every patch of the imported MLOps history (all commits incl. merges), as one text file
        log = Path(tmp) / "mlops-history.patch"
        log.write_text(
            git("log", "-p", "-m", "--format=commit %H", _subtree_split_parent()), "utf-8"
        )
        history = _findings([str(log)])
    assert not tree, f"secrets in the imported tree: {_summary(tree)}"
    assert not history, f"secrets in the imported history: {_summary(history)}"
