# CI gates (ASDLC v2)

Workflows live at the repo root (`.github/workflows/`); nested per-repo workflows were dead and are ported into `services-ci.yml`.

## Jobs (`asdlc-gates.yml`, on `pull_request` and `workflow_dispatch`, `contents: read`)

| Job | What it runs | Blocks |
|---|---|---|
| specs | `scripts/asdlc/spec_job.sh` (also used by `ci_local.sh`): `openspec validate --strict`, `spec_lint.py`, `spec_sync.py` per changed `openspec/changes/<id>` — `--strict` only when every task in `tasks.md` is checked or the change is moved to `archive/` in the PR (spec-only PRs get a non-strict report); names not matching `^[a-z0-9][a-z0-9-]*$` are skipped with a warning (PR-controlled, never interpolated into `run:`); `cross_change_check.py` per changed `plans/*/slices.yaml` | yes |
| features | `make -C platform-e2e features-check` | yes |
| repo | `repo_doctor.py`, `validate_plan.py` (each `plans/*/` with slices; task packets required only once `LOCK.md` is `status: approved`), `asdlc_config.py --check`, `check_gates.py` | yes |
| proto | `buf lint`; `buf breaking` against the PR base | yes |
| tests-asdlc | pytest for `scripts/asdlc/tests`, `platform-e2e/scripts/tests`, guard-hook test, red_first test | yes |
| tdd | `red_first.sh --only go,python` (base = merge-base with PR base), `no_self_mock.py --base`. Installs deps first: `go mod download` per changed Go module, `uv export --frozen --all-extras --dev` for uv-managed dirs (`uv.lock`), else `pip install -r requirements.txt` / `-e .[dev]`, for changed Python test dirs (not `platform-e2e`); buf is installed so red_first can regenerate gitignored proto code (`buf generate`, nearest `buf.gen.yaml`) in its base and head worktrees. A test changed only to follow a dependency bump is declared with a commit trailer `Red-First-Exempt: <glob> -- <reason>`: it skips the base run but must pass at head, and is reported EXEMPT. **TypeScript tests are skipped in CI** (no `node_modules`; a notice is printed): run `scripts/asdlc/red_first.sh` locally for TS. Fails on uncommitted test/code changes (`--allow-dirty`) and on impl changes with no tests (`--allow-empty`); `metrics/**`, docs, plans and specs are not implementation | yes |
| security | gitleaks (`.gitleaksignore` fingerprints), trivy fs (HIGH/CRITICAL, ignore-unfixed; reviewed false positives in `.trivyignore.yaml`), semgrep | gitleaks, trivy yes; semgrep advisory (`continue-on-error`: rule pack not yet baselined on this polyrepo; promote once clean) |
| mutation | advisory (W5-4), only on manual dispatch or PR label `mutation`: mutmut (team-ai), gremlins (Go), Stryker (team-frontend); threshold 40%, score in job summary | never |
| security-review | `anthropics/claude-code-action` `/security-review`, only when secret `ANTHROPIC_API_KEY` exists | never |

`services-ci.yml`: team-ai (lint/test, uv pytest + eval gate), platform-recsys, platform-modelserve, with `paths:` filters.

## Run locally

- Default (no act): `scripts/asdlc/ci_local.sh [job...]` (jobs: specs features repo proto tests-asdlc tdd security). Prints a PASS/FAIL/SKIP table; `BASE=<ref>` sets the PR base (default `main`).
- Missing tools are SKIPped with a message. gitleaks/trivy/semgrep via official images: `ci_local.sh --docker-tools security`.
- act: `act -l` lists jobs; `act pull_request -j features --container-architecture linux/amd64`. Jobs needing `fetch-depth` history (tdd, proto, specs) are better run with `ci_local.sh`.

## Human approval points

- Pushing the workflows and marking the jobs as **required checks** in branch protection (repo settings) is a human step; nothing here is enforced until then.
- Merging a PR is the human approval; CI only gates it.

## Cannot run in CI

- Full-stack e2e (docker compose + Playwright against the running stack): run locally with `make -C platform-e2e` targets; CI only checks the manifests (`features-check`), not the scenarios.
- Live-cluster checks (kind/ArgoCD), and anything needing real secrets.
- `security-review` and Anthropic-backed engines need `ANTHROPIC_API_KEY`.
