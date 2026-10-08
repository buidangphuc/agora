## 1. Code track (team-ai; one agent, worktree)

- [ ] 1.1 Router: per-target breakers (closed/open/half-open with a single probe), failure classification (`FailureKind`), `eligible_targets`; verify unit tests with a fake clock: 3 targets, 5xx/429/timeout/connection count and 400/404 do not, probe success closes, probe failure re-opens, concurrent requests skip during the probe
- [ ] 1.2 Streamer: per-request attempt loop, memoised models, first-token timeout, `LLM_MAX_ATTEMPTS`, deadline cap from `context.time_remaining()`, no switch after the first chunk, outcome reporting; verify unit tests with scripted fake models for every case in the spec's routing and timeout requirements
- [ ] 1.3 `StreamChat` error mapping with fixed messages, plus logging of the original exception with the request id; verify servicer tests for `UNAVAILABLE`, `RESOURCE_EXHAUSTED` and `DEADLINE_EXCEEDED`, and that no exception text reaches the client
- [ ] 1.4 Usage capture and the structured usage log line; quota reserve (server-minted id) / finalize / refund around the call when `QUOTA_ENABLED`, with resource `chat.reply` and the `QUOTA_CHAT_*` policy; verify tests for usage with and without provider usage, exhausted (no model call), refund on pre-chunk failure, finalize after a partial reply, and disabled (no calls)
- [ ] 1.5 gRPC rate limit covers `StreamChat` and `ShoppingAssistant`; the strict-ENV boot refusal for the memory backend; verify interceptor tests and the config validation test
- [ ] 1.6 System prompt (Langfuse, then `CHAT_SYSTEM_PROMPT`, then static) and `ChatSessionStore` (Redis or in-process; bounded; TTL; appended only on completion); verify unit tests for each history rule and with Langfuse disabled or failing
- [ ] 1.7 `RedactionPolicy.for_llm_input` with the VN phone and ID patterns, applied to model input; `LLM_TRACE_CONTENT` for logs and traces (`full` refused in strict ENV); `AIAssistantService` logs redacted; verify the redaction table tests, including prices and order numbers left intact, and loguru capture tests
- [ ] 1.8 Langfuse trace config with session, user and request ids and the target tag; verify unit tests that the callback is attached when enabled and absent when disabled
- [ ] 1.9 Eval: `chat_resilience` suite against the real streamer; unscored cases count as failures; a judge case without a judge exits non-zero naming the case; `EVAL_EXTRA_CASES`; verify `make eval` and a test for the denominator
- [ ] 1.10 Update `.env.example`, README settings and `make check`; verify `make check` and `make test` are green

## 2. E2E track (platform-e2e; team-ai FEATURES.yaml)

- [ ] 2.1 `platform-e2e/fakes/llm_fake` (D8) with its own unit tests, plus `compose/llm-fake.override.yaml`; add the overlay to the stack wrapper notes in `platform-e2e/compose/README`; verify the fake answers directives with curl
- [ ] 2.2 One `team-ai/FEATURES.yaml` acceptance line per scenario, starting as `planned`; verify `features-check`
- [ ] 2.3 `ai/llm_routing.feature` (healthy, 429 fallback, chain order, break after the first chunk, exhausted, hung primary, bounded attempts) and `ai/llm_breaker.feature` (destructive); verify against the stack running the overlay
- [ ] 2.4 `ai/llm_quota_and_limits.feature` (usage log, exhausted, refund, per-principal limit) and the team-ai rate-limit boot guard in `ops/boot_guard_ai.feature` (destructive); verify they pass
- [ ] 2.5 `ai/llm_prompt_and_privacy.feature` (system prompt, history, bound, failed reply, phone/ID masking, prices kept, assistant logs) and `ai/llm_tracing.feature`; verify they pass
- [ ] 2.6 `ai/llm_eval_gate.feature` running `make -C team-ai eval` as a black box (pass, and a judge case without a judge); verify they pass
- [ ] 2.7 The existing assistant and AI scenarios stay green on the overlay; verify `ai/` and `ai_and_cockpit/` features

## 3. Review and verify

- [ ] 3.1 `auth-scope-reviewer` (quota and rate limit keyed by the forwarded principal; no client-controlled ids) and `contract-boundary-reviewer` over the change's commits; verify no blocking finding remains
- [ ] 3.2 Gate: `openspec validate ai-path-resilience --strict`, `features.py --strict`, `spec_sync.py ai-path-resilience --strict`, `repo_doctor`
- [ ] 3.3 Full e2e suite green twice in the parallel lane plus the destructive lane, with the overlay on; every new flake root-caused

## 4. Archive

- [ ] 4.1 `openspec archive ai-path-resilience`; retire `llm-path-resilience`, and retire `gateway-and-ai-hardening` after moving its recommendation items to the AI-first `recs-serving-safeguards` change; verify `openspec list`
