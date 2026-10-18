# Project facts for agents

team-ai: the AI service of the agora platform. Redis and recommendations are mandatory; gRPC, the rate limit, quota and taxonomy persistence are always on. Four switches remain: `CHAT_BACKEND`, `RAG_ENABLED`, `AI_USE_SCOPE_REQUIRED`, `DOCS_ENABLED`.

## Feedback loops
<!-- read by: diagnosing-bugs, tdd -->
- Unit/integration: `uv run --all-extras pytest -q` (offline: fakeredis, memory recommendations and RAG seams, `GRPC_PORT=0`)
- Eval gate: `uv run --env-file .env.example --all-extras python -m scripts.run_eval --min-score 0.8`
- Real-infra tests are env-gated. Fakes hid real bugs (stream trimming, `XDEL`), so adapter/lifecycle changes also run these:
  - `REAL_REDIS_URL=redis://localhost:6379/15 uv run --all-extras pytest -q -p tests.plugins.real_redis tests/unit/modules/test_rate_limit.py tests/unit/modules/test_quota.py` (the plugin turns every `FakeRedis` into a client on that database; point it at other fakeredis-based test files for the recommendation, session or taxonomy stores)

## Checks
<!-- read by: pre-commit-ci. Mirror CI exactly. -->
- Source: .github/workflows/services-ci.yml, job `team-ai-uv` (last read: 2026-10-18)
- `uv sync --dev --all-extras`
- `uv run --all-extras python -m scripts.gen_proto` (generates the git-ignored gRPC stubs; run before pyright/pytest)
- `uv run ruff format --check app tests scripts`
- `uv run ruff check app tests scripts`
- `uv run --all-extras pyright`
- `uv run --all-extras python -m scripts.check_env_example` (`.env.example` matches Settings)
- `SKIP=commitizen uv run pre-commit run --all-files --hook-stage pre-commit`
- `uv run --all-extras pytest -q --cov=app --cov-report=term-missing --cov-fail-under=80`
- `uv run --env-file .env.example --all-extras python -m scripts.run_eval --min-score 0.8`
- `uv run --no-dev --env-file .env.example python -c "import main; print('ok')"` (after `uv sync --frozen --no-dev` and `uvx --from grpcio-tools python -m scripts.gen_proto`)
- `REAL_REDIS_URL=redis://localhost:6379/15 uv run --all-extras pytest -q -p tests.plugins.real_redis tests/unit/modules/test_rate_limit.py tests/unit/modules/test_quota.py` (needs a Redis on :6379)

## Tracker
<!-- read by: to-tickets -->
- None: agora has no tracker. Changes are planned as OpenSpec changes under `../openspec/changes/` (archived to `openspec/changes/archive/`, specs in `../openspec/specs/`).

## Docs
<!-- read by: domain-modeling, research -->
- Architecture: `docs/agents/architecture.md` (read before touching bootstrap/platform/business/transport boundaries or adding a feature)
- AI: `docs/agents/ai-engineering.md` (LLM/prompt/RAG/eval/completions)
- ADRs: `docs/adr/`
- Usage: `docs/usage.md`

## Hot paths
<!-- read by: everyone. Things that look alike but must not be confused. -->
- `app.state.resources` is the deliberate controller seam: views and servicers read services there.
- `app/core/telemetry.py` (no OTel import) vs `app/core/telemetry_otel.py` (optional extra).
- `.env.example` is checked against Settings by `scripts/check_env_example.py`.
- `app/transport/grpc/_pb/` is generated from `proto/` by `scripts/gen_proto.py` and git-ignored; regenerate it after pulling proto changes.
- `serve_grpc` (`app/bootstrap/grpc.py`) is not a `BootstrapAddon`: it starts after the services are composed and stops before resources close.
- Env keys that Settings no longer has (old gitops feature flags) are ignored when they come from the OS environment, and fail validation in a `.env` file.
