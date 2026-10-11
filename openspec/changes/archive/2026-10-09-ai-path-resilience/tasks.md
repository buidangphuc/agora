## 1. Code track (team-ai; one agent, worktree)

- [x] 1.1 Router: per-target breakers (closed/open/half-open with a single probe), failure classification (`FailureKind`), `eligible_targets`; verify unit tests with a fake clock: 3 targets, 5xx/429/timeout/connection count and 400/404 do not, probe success closes, probe failure re-opens, concurrent requests skip during the probe
- [x] 1.2 Streamer: per-request attempt loop, memoised models, first-token timeout, `LLM_MAX_ATTEMPTS`, deadline cap from `context.time_remaining()`, no switch after the first chunk, outcome reporting; verify unit tests with scripted fake models for every case in the spec's routing and timeout requirements
- [x] 1.3 `StreamChat` error mapping with fixed messages, plus logging of the original exception with the request id; verify servicer tests for `UNAVAILABLE`, `RESOURCE_EXHAUSTED` and `DEADLINE_EXCEEDED`, and that no exception text reaches the client
- [x] 1.4 Usage capture and the structured usage log line; quota reserve (server-minted id) / finalize / refund around the call when `QUOTA_ENABLED`, with resource `chat.reply` and the `QUOTA_CHAT_*` policy; verify tests for usage with and without provider usage, exhausted (no model call), refund on pre-chunk failure, finalize after a partial reply, and disabled (no calls)
- [x] 1.5 gRPC rate limit covers `StreamChat` and `ShoppingAssistant`; the strict-ENV boot refusal for the memory backend; verify interceptor tests and the config validation test
- [x] 1.6 System prompt (Langfuse, then `CHAT_SYSTEM_PROMPT`, then static) and `ChatSessionStore` (Redis or in-process; bounded; TTL; appended only on completion); verify unit tests for each history rule and with Langfuse disabled or failing
- [x] 1.7 `RedactionPolicy.for_llm_input` with the VN phone and ID patterns, applied to model input; `LLM_TRACE_CONTENT` for logs and traces (`full` refused in strict ENV); `AIAssistantService` logs redacted; verify the redaction table tests, including prices and order numbers left intact, and loguru capture tests
- [x] 1.8 Langfuse trace config with session, user and request ids and the target tag; verify unit tests that the callback is attached when enabled and absent when disabled
- [x] 1.9 Eval: `chat_resilience` suite against the real streamer; unscored cases count as failures; a judge case without a judge exits non-zero naming the case; `EVAL_EXTRA_CASES`; verify `make eval` and a test for the denominator
- [x] 1.10 Update `.env.example`, README settings and `make check`; verify `make check` and `make test` are green

## 2. E2E track (platform-e2e; team-ai FEATURES.yaml)

- [x] 2.1 `platform-e2e/fakes/llm_fake` (D8) with its own unit tests, plus `compose/llm-fake.override.yaml`; add the overlay to the stack wrapper notes in `platform-e2e/compose/README`; verify the fake answers directives with curl
- [x] 2.2 One `team-ai/FEATURES.yaml` acceptance line per scenario, starting as `planned`; verify `features-check`
- [x] 2.3 `ai/llm_routing.feature` (healthy, 429 fallback, chain order, break after the first chunk, exhausted, hung primary, bounded attempts) and `ai/llm_breaker.feature` (destructive); verify against the stack running the overlay
- [x] 2.4 `ai/llm_quota_and_limits.feature` (usage log, exhausted, refund, per-principal limit) and the team-ai rate-limit boot guard in `ops/boot_guard_ai.feature` (destructive); verify they pass
- [x] 2.5 `ai/llm_prompt_and_privacy.feature` (system prompt, history, bound, failed reply, phone/ID masking, prices kept, assistant logs) and `ai/llm_tracing.feature`; verify they pass
- [x] 2.6 `ai/llm_eval_gate.feature` running `make -C team-ai eval` as a black box (pass, and a judge case without a judge); verify they pass
- [x] 2.7 The existing assistant and AI scenarios stay green on the overlay; verify `ai/` and `ai_and_cockpit/` features

## 3. Review and verify

- [x] 3.1 `auth-scope-reviewer` (quota and rate limit keyed by the forwarded principal; no client-controlled ids) and `contract-boundary-reviewer` over the change's commits; verify no blocking finding remains
- [x] 3.2 Gate: `openspec validate ai-path-resilience --strict`, `features.py --strict`, `spec_sync.py ai-path-resilience --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane, with the overlay on; every new flake root-caused

## Evidence (2026-10-09)

- Code (team-ai): 3d108ae0, 6fe90122, eb57db62, 23ba7f77, ecbd6b02, ec35500a, a9885314, 0531f3e7, 21f71f6d, ac48c315, 7e32656c; integration fixes 2c72f41e (anonymous keyed by client IP, empty id is anonymous, session_id cap, llm_router outside local needs rate limit + quota) and the `ai`-extra boot guard.
- e2e infra: fake provider `platform-e2e/fakes/llm_fake` (97288115) with its unit tests, overlays `llm-fake.override.yaml`, `llm-fake-langfuse.override.yaml`, `llm-fake-breaker.override.yaml`.
- e2e: spec_sync 26/26. apr scenarios 28/28 non-destructive (with the existing AI ones) and 6/6 destructive. Full suite 432/432 and 431/432 (`-n 4`), then destructive lane 48/48; the one parallel failure was the rate-limit burst straddling a sliding-window boundary, fixed by starting bursts inside a window.
- Defects found on the live stack, each fixed in its own commit:
  - the team-ai image had no `ai` extra, so `llm_router` died with ModuleNotFoundError on every chat (now a boot error; the e2e overlay builds with `recs ai`);
  - breakers are process-wide, so parallel scenarios opened each other's fallbacks (standing threshold 1000, breaker scenarios use their own overlay);
  - the gateway's per-IP collector limit (from port-edge-authz-residuals) throttled every e2e browser sharing the docker host IP (local compose raises it; the flood scenario uses a throwaway default-limit gateway).
- Spec corrected where the code was right: team-ai's environment setting is `ENVIRONMENT` (no staging value).
- Reviews:
  - contract-boundary: no blocking findings. Follow-ups:
    - `app/modules/ai/llm/testing.py` (scripted models for the eval) lives in the app tree;
    - pre-existing: root compose gives team-ai team-domain's listing DB credentials (unused while `DATABASE_ENABLED=false`, but a Rule 3 risk once a Postgres quota backend is enabled).
  - auth-scope:
    - B1: anonymous chat on the paid path was unmetered and shared one bucket. Fixed in 2c72f41e and pinned by "Production refuses an unmetered LLM chat path".
    - Remaining follow-ups:
      - quota and limiter fail open on store errors;
      - an early client cancel is refunded after the model was called;
      - redaction does not cover names, addresses or card numbers;
      - the in-memory session store has no size-bounded sweep;
      - tracebacks bypass `LLM_TRACE_CONTENT`.

## 4. Archive

- [x] 4.1 `openspec archive ai-path-resilience`; retire `llm-path-resilience`, and retire `gateway-and-ai-hardening` after moving its recommendation items to the AI-first `recs-serving-safeguards` change; verify `openspec list`
