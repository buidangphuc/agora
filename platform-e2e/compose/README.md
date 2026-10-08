# Compose overlays for e2e

Each overlay is layered on the stack with an extra `-f` and keys services by name only (no
`container_name`), so it works under any compose project. Run `docker compose` from the
workspace root so relative build contexts resolve. The stack wrapper
(`dc-agora-ov.sh`, under `~/Library/Caches/ai-first-runs/agora-stack/`) is a plain
`docker compose -p agora -f docker-compose.yaml -f <isolation override> -f <overlays...> "$@"`; add
the overlays you need to its `-f` list.

| Overlay | Used by | Effect |
|---|---|---|
| `order-inventory.override.yaml` | order / inventory features | 20 s reservation TTL, 2 s sweep |
| `payment-ledger.override.yaml` | payment ledger features | short ledger windows |
| `search-tombstones.override.yaml` | search read-model features | 30 s tombstone TTL |
| `llm-fake.override.yaml` | `ai/llm_*.feature`, `ops/boot_guard_ai.feature` | adds the `llm-fake` provider, switches team-ai to `CHAT_BACKEND=llm_router` against it |
| `llm-fake-langfuse.override.yaml` | `ai/llm_tracing.feature` only | turns Langfuse on in team-ai, pointed at the fake's ingestion endpoint |

## `llm-fake.override.yaml` (change ai-path-resilience)

`platform-e2e/fakes/llm_fake/` is a stdlib-only, OpenAI-compatible fake provider that can be
told per request how to fail (`[[fake primary=429 fb1=ok fb2=ok]]`, modes `ok|429|500|400|hang|break2`),
records every request body (`GET /_requests?contains=`), and records Langfuse / OTLP ingestion
(`GET /_ingested?contains=`). The overlay builds and runs it as service `llm-fake`, publishes it
on `127.0.0.1:${LLM_FAKE_HOST_PORT:-18099}` and switches team-ai to:

| Setting | Value |
|---|---|
| `CHAT_BACKEND` / `CHAT_MODEL` / `CHAT_FALLBACK_MODELS` | `llm_router` / `openai:primary` / `openai:fb1,openai:fb2` |
| `OPENAI_BASE_URL` / `OPENAI_API_KEY` | `http://llm-fake:8099/v1` / `fake` |
| `LLM_FIRST_TOKEN_TIMEOUT_SECONDS` / `LLM_MAX_ATTEMPTS` | 2 / 3 |
| `LLM_BREAKER_THRESHOLD` / `LLM_BREAKER_COOLDOWN_SECONDS` | 2 / 5 |
| `CHAT_SYSTEM_PROMPT` / `CHAT_HISTORY_MAX_TURNS` | a fixed e2e prompt / 4 |
| `QUOTA_ENABLED` / `QUOTA_BACKEND` / `QUOTA_CHAT_REPLIES_PER_WINDOW` | true / memory / 15 (design says 30, see the file header) |
| `GRPC_RATE_LIMIT_ENABLED` / `RATE_LIMIT_PRINCIPAL_PER_MINUTE` | true / 20 |

Bring it up (stack already running, wrapper already carrying the overlay):

    DC=~/Library/Caches/ai-first-runs/agora-stack/dc-agora-ov.sh
    $DC up -d --build llm-fake
    $DC up -d --no-deps team-ai        # recreates team-ai with the new env
    curl -s localhost:18099/healthz    # {"status": "ok"}

The tests find the fake at `LLM_FAKE_URL` (default `http://localhost:18099`) and team-ai's logs in
the container named by `AI_CONTAINER` (default `agora-team-ai-svc`).

### Langfuse variant

Only the trace scenario needs it, and it applies it itself: `llm-fake-langfuse.override.yaml` is
passed as an extra `-f` to `up -d --no-deps team-ai`, and teardown recreates team-ai without it.
It is never part of the standing stack, which is why the scenario is `@destructive`.

### Unit tests for the fake

    platform-e2e/.venv/bin/pytest -p no:cacheprovider platform-e2e/unit/test_llm_fake.py -q
