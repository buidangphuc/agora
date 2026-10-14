"""CI workflow checks (spec: model-registry, CI scenarios)."""
from pathlib import Path

import yaml

WORKFLOW = Path(__file__).resolve().parents[4] / ".github" / "workflows" / "services-ci.yml"
LIB_PATH = "platform-core/packages/py-modelregistry"
LIB_GLOB = f"{LIB_PATH}/**"
JOB = "py-modelregistry"


def _workflow() -> dict:
    assert WORKFLOW.is_file(), f"missing {WORKFLOW}"
    return yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))


def _triggers(wf: dict) -> dict:
    # YAML 1.1 parses the bare key `on` as boolean True.
    return wf.get("on", wf.get(True)) or {}


def _job() -> dict:
    jobs = _workflow().get("jobs") or {}
    assert JOB in jobs, f"job {JOB!r} missing; have {sorted(jobs)}"
    return jobs[JOB]


def _run_commands(job: dict) -> list[str]:
    return [s["run"] for s in job.get("steps", []) if "run" in s]


def test_job_and_path_filter_present():
    """Scenario: Job triggers on library changes."""
    wf = _workflow()
    triggers = _triggers(wf)
    for event in ("push", "pull_request"):
        assert LIB_GLOB in (triggers.get(event) or {}).get("paths", []), (
            f"{event}.paths lacks {LIB_GLOB}"
        )
    assert JOB in wf["jobs"]


def test_job_runs_pytest_without_continue_on_error():
    """Scenario: A failing test fails the job."""
    job = _job()
    steps = job.get("steps", [])
    assert not job.get("continue-on-error"), "job has continue-on-error"
    assert not any(s.get("continue-on-error") for s in steps), "a step has continue-on-error"

    pythons = [
        str(s["with"]["python-version"])
        for s in steps
        if str(s.get("uses", "")).startswith("actions/setup-python") and "with" in s
    ]
    assert pythons == ["3.10"], f"expected python 3.10, got {pythons}"

    workdir = (job.get("defaults") or {}).get("run", {}).get("working-directory")
    for cmd in ("ruff check", "mypy", "pytest"):
        step = next((s for s in steps if cmd in s.get("run", "")), None)
        assert step is not None, f"no step runs {cmd!r}"
        step_dir = step.get("working-directory", workdir)
        assert step_dir == LIB_PATH, f"{cmd!r} runs in {step_dir!r}, not {LIB_PATH}"
