# Project facts for agents

## Feedback loops
<!-- read by: diagnosing-bugs, tdd -->
- **Go service unit**: `(cd team-<svc> && go test ./...)` — fastest red/green for a team-* service; needs generated stubs (`team-*/generated`; team-gateway: `buf generate --template buf.gen.local.yaml`).
- **Frontend unit**: `(cd team-frontend && npx vitest run <file>)` — colocated `*.test.ts(x)`; needs `node_modules` and `npm run proto` stubs.
- **platform-featurestore**: `(cd platform-featurestore && uv run --frozen --all-extras pytest -q)` — the per-repo `.venv` has no pytest.
- **platform-recsys**: from `platform-recsys/`: `uv venv <scratch> && uv pip install -p <scratch> -r requirements-dev.txt -e .` (pulls py-modelregistry from `../platform-core/packages/py-modelregistry`, so run it from that dir), then `make -C platform-recsys compile lint check-env test-host PY=<scratch>/bin/python` — CI uses Python 3.10; Spark tests (`make test`, `make eval`) need Java 17 on `PATH`.
- **py-modelregistry**: `cd platform-core/packages/py-modelregistry && pip install -e '.[dev]' && pytest -q` — offline, local MLflow file store; CI also runs `ruff check .` and `mypy src` (py3.10).
- **platform-finetune**: `cd platform-finetune && uv pip install -e '.[dev,cpu]' 'mlflow>=3.2,<3.3' && pytest -q` — CPU, no GPU, no dataset download; the mlflow pin matches py-modelregistry's mlflow-skinny (pip's resolver hangs on the `dev` extra — use uv).
- **e2e (full stack)**: `(cd platform-e2e && HEADLESS=true .venv/bin/python -m pytest -q tests/e2e/features/<area>/<file>.feature)` — the stack runs from `../agora-deps` (compose project `agora`), not this tree; rebuild one service before testing worktree code (see memory `reference-local-e2e-stack`).
- **Spec↔e2e for a change**: `platform-e2e/.venv/bin/python platform-e2e/scripts/spec_sync.py <change> [--strict]` — uncovered scenarios and tasks/FEATURES/.feature cross-check.

## Checks
<!-- read by: pre-commit-ci. Mirror CI exactly. -->
- Source: `.github/workflows/asdlc-gates.yml`, `.github/workflows/services-ci.yml`, `docs/ci.md` (last read: 2026-10-12; services-ci re-read 2026-10-14)
- `scripts/asdlc/ci_local.sh [specs features repo proto tests-asdlc tdd security]` — runs every asdlc-gates job locally; `BASE=<ref>` (default `main`), `ALLOW_DIRTY=1` for tdd with an untracked worktree, `--docker-tools` for gitleaks/trivy/semgrep.
  - specs: `scripts/asdlc/spec_job.sh <merge-base>` (openspec validate --strict, spec_lint, spec_sync, cross_change_check) — lint
  - features: `make -C platform-e2e features-check` — lint
  - repo: `repo_doctor.py`, `validate_plan.py plans/<id>`, `asdlc_config.py --check`, `check_gates.py` — lint
  - proto: `buf lint` + `buf breaking` vs base in `platform-core/packages/proto` — lint
  - tests-asdlc: pytest `scripts/asdlc/tests platform-e2e/scripts/tests`, `.claude/hooks/tests/test_guard_tests.sh`, `scripts/asdlc/tests/test_red_first.sh` — test
  - tdd: `red_first.sh --only go,python --base <merge-base>`, `no_self_mock.py --base origin/<base>` — test (needs `Red-First-Exempt:` trailer for guard tests green at base)
  - security: gitleaks, trivy fs (HIGH/CRITICAL), semgrep (advisory) — security
- services-ci (path-filtered):
  - team-ai: `uv sync --dev --all-extras && uv run --all-extras python -m scripts.gen_proto && uv run --all-extras pytest -q && uv run --all-extras python -m scripts.run_eval --min-score 0.8` — test
  - platform-recsys (py3.10): `make compile lint check-env test-host eval` — lint/test
  - platform-modelserve (py3.12): `make lint test` — lint/test
  - py-modelregistry (py3.10): `pip install -e '.[dev]' && ruff check . && mypy src && pytest -q` — lint/types/test
  - platform-finetune (py3.11): `uv pip install --system -e '.[dev,cpu]' 'mlflow>=3.2,<3.3' && python -m pytest tests -q` — test
  - Path filter also covers `platform-finetune/**` and `platform-core/packages/py-modelregistry/**`.
- Local-only (no CI job; run for touched repos): Go `go vet ./... && go test ./...`; team-frontend `npx vitest run`, `npm run typecheck`, `npx prettier --check <changed files>` (ESLint config is broken, not in CI).
- Cannot run in CI: full-stack e2e, live-cluster checks, anything needing secrets (`ANTHROPIC_API_KEY`).

## Tracker
<!-- read by: to-tickets -->
- System: local-only — work items are OpenSpec changes (`openspec/changes/<id>/tasks.md`) grouped by initiatives (`plans/<id>/`); PRs on GitHub `buidangphuc/agora`.

## Docs
<!-- read by: domain-modeling, research -->
- Glossary: N/A (none yet) | ADRs: `platform-core/docs/ADR/` | Architecture: `platform-core/docs/` | Process: `docs/ASDLC-v2.md`, `docs/ci.md` | Runbooks: `docs/runbooks/` | Specs: `openspec/specs/`

## Hot paths
<!-- read by: everyone. Things that look alike but must not be confused. -->
- `.github/workflows/` at repo root is the only live CI; nested per-repo workflows are dead.
- Local e2e stack is built from `../agora-deps`, not this checkout — a stale image gives a wrong-reason red.
- `.claude/worktrees/<change>/` lacks gitignored artifacts (stubs, node_modules, `platform-e2e/.venv` symlink — never commit it); red_first temp worktrees report false "red at head" for stub-dependent tests.
- `plans/` (initiatives, ASDLC) vs `plan-analytics/`, `plan-mlops/` (older free-form plans).
- `order_facts` is the only revenue source; never sum money from `tracking_events` or GA4.
- The platform-recsys image builds from the monorepo root (`docker build -f platform-recsys/Dockerfile ..`; compose `context: .`) so it can copy py-modelregistry; building from `platform-recsys/` fails.
- The platform-finetune image builds from `platform-finetune/` and gets py-modelregistry only through the compose `additional_contexts` entry `modelregistry`; a plain `docker build` still succeeds but has no registry library, and `python main.py run` fails closed on import.
- MLflow stores metric names with `@` encoded as `_at_` (`ndcg@10` → `ndcg_at_10`); py-modelregistry decodes on read, so callers use `@` and never put `_at_` in a metric name.
