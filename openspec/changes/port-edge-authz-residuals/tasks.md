## 1. Code track (one agent per area, worktree each)

- [x] 1.1 team-gateway: full `connect.Interceptor` chain covering streams (request id header, fail-closed token, admin policy, shared limiter, one log line), `WithReadMaxBytes(STREAM_MAX_REQUEST_BYTES)` on the chat handler; verify unit tests: an invalid token on a stream gives `unauthenticated` with no upstream call, a stream past the burst gives `resource_exhausted`, an oversized request gives `resource_exhausted`, and the request id is echoed
- [x] 1.2 team-gateway: `edgeHTTP` middleware on `/api/track` (separate `TRACK_RATE_LIMIT_*` limiter, 429 before produce) and `/api/admin/metrics` (shared limiter), request id echo, log line; verify unit tests that a 429 produces nothing and that a logged-in visitor is keyed by user
- [x] 1.3 team-gateway: `callAI` (one attempt, `AI_CALL_TIMEOUT_SECONDS`) for the four AI procedures, plus `attempts` on `edge.request`; verify a unit test with a fake upstream that returns `Unavailable` and is called exactly once
- [x] 1.4 team-gateway: `EDGE_REFLECTION_ENABLED` (default off) and the strict-ENV refusal in `config.Validate`; verify unit tests for the 404 when off and for the validate error naming the flag
- [x] 1.5 team-gateway: dial `ConnectParams` bounded by `DIAL_TIMEOUT_SECONDS` (default 2); verify a unit test that the setting reaches the dial options
- [x] 1.6 team-gateway: real `TestEnvExampleInSync` and an `.env.example` updated for every new setting; add `ForceFailSaga` to `adminProcedures` and update the pinning test; verify `make check`
- [x] 1.7 team-order: `ForceFailSaga` admin-only; `CreateOrder` USER-only, checked before the kill-switch and any reservation; verify handler tests for buyer, admin, service and anonymous on both RPCs, then `go vet ./... && go test ./...`
- [x] 1.8 team-identity: refuse `PASSWORD_RESET_EXPOSE_TOKEN` under strict ENV (staging and prod); verify config tests and `make check`
- [x] 1.9 team-payment: `listing.write` required on `RequestPayout`/`RequestWalletPayout`; verify handler tests for a buyer (denied, no payout row) and a seller (allowed)
- [x] 1.10 platform-gitops: gateway `ENV` in the staging/prod overlays; team-ai `networkPolicy` in `envs/services/team-ai.yaml` and `argocd/apps/team-ai.yaml`; fix the vault app comment; verify `helm template` for team-gateway (staging, prod) and team-ai shows `ENV` and the NetworkPolicy
- [x] 1.11 Root/platform-core: delete `deploy/vault/bootstrap-vault.sh` and update `deploy/README.md`; pin MinIO; port `check_documented_env` into `scripts/repo_doctor.py` and fix every real violation (one commit each); verify `grep -rn bootstrap-vault . --exclude-dir=.git --exclude-dir=openspec` is empty, `docker compose config` resolves the pinned MinIO, and `repo_doctor --root .` passes

## 2. E2E track (platform-e2e; owning repos' FEATURES.yaml)

- [x] 2.1 Add one FEATURES.yaml acceptance line per scenario in the owning repo (gateway, order, payment, identity, gitops, root), with `covered_by` naming a real `.feature` scenario; verify `make -C platform-e2e features-check`
- [x] 2.2 `security/edge_streams.feature`: invalid token, request id echo, per-caller rate limit, oversized request; verify against the stack
- [x] 2.3 `security/edge_http_routes.feature`: collector request id (parallel), beacon flood throttled (destructive lane, `analytics.events` checked for the markers); verify both pass
- [x] 2.4 `security/edge_ai_and_reflection.feature`: reflection 404 (parallel), MagicListing one attempt with team-ai stopped (destructive); add the gateway and identity strict-ENV cases to `ops/boot_guards.feature` (destructive); verify they pass
- [x] 2.5 `ops/upstream_recovery.feature` (destructive): recreate team-payment and assert recovery within 5 s; verify it passes 3 runs in a row
- [x] 2.6 Order and payment: buyer `ForceFailSaga` gets 403 (`edge_route_policy.feature`); switch `oic_saga_view.feature` and `group_a_steps.py` to the admin; SERVICE-token `CreateOrder` (`order/checkout_principal.feature`); payouts (`payment/payout_scope.feature`); verify they pass and the existing saga scenarios stay green
- [x] 2.7 `ops/gitops_render.feature`: `helm template` assertions for the gateway `ENV` and the team-ai NetworkPolicy; `ops/repo_doctor.feature`: a temp-copy ghost env var fails, and the real workspace passes; verify both pass

## 3. Review and verify

- [x] 3.1 Run `auth-scope-reviewer` and `contract-boundary-reviewer` over the change's commits; verify no blocking finding remains, each fix in its own commit
- [x] 3.2 Gate: `openspec validate port-edge-authz-residuals --strict`, `features.py --strict`, `spec_sync.py port-edge-authz-residuals --strict`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane (`-n 4`) plus the destructive lane; every new flake root-caused

## Evidence (2026-10-08)

- Code: gateway 58a26235, 7e963848, de2381c5; order 758f0338; identity fa1e5e54; payment 7ae37b93; gitops 1d2cae7d; root/platform-core 67b8be4a, 25d21778, 0db6235b.
- Recovery (D4): bounded backoff alone (`DIAL_TIMEOUT_SECONDS=2`, base 200 ms) met the 5 s bound; no `round_robin` fallback needed.
- e2e: spec_sync 34/34. Full suite 412 passed and 412 passed (`-n 4`, not destructive), then destructive lane 41/42; the one failure ("A seller can still request a payout") was a test reading the bank-payout list (`ListPayoutHistory`) instead of the wallet ledger that `RequestWalletPayout` writes; spec wording and step fixed, the scenario then passed.
- Defects found while integrating, each fixed in its own commit: e2e switched only the auth client to the admin token (84d7b149); the doctor's workspace copy tripped on a socket (84d7b149); a UI fixture still force-failed as the buyer (22b9a94e).
- Spec corrected where the code was right: a Connect stream reports `unauthenticated` in its end-of-stream error (HTTP 200); the read cap covers every `ChatService` RPC.
- Reviews: auth-scope and contract-boundary found no blocking issue. Follow-ups:
  - team-order still accepts the unused `order.admin` scope;
  - stream tokens are checked only at stream open (a revoked user keeps an open stream until it closes; ties to the deferred stream duration cap);
  - unverifiable tokens are not rate limited (unchanged behaviour);
  - strict-ENV guards match exact values, so a typo such as `prd` disables them;
  - the cockpit ignores the session cookie.

## 4. Archive

- [ ] 4.1 `openspec archive port-edge-authz-residuals`; retire `service-authz-hardening` and `authz-hardening-wave2`; trim `gateway-and-ai-hardening` to its team-ai requirements (moved to `ai-path-resilience`); verify `openspec list` and the folded specs
