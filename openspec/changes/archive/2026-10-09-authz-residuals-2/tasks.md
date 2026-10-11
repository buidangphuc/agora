## 1. Code track (one agent per repo, worktree each)

- [x] 1.1 team-identity: add `order.admin` to `roleScopes[RoleAdmin]` only; verify authz tests: admin has `admin` and `order.admin`, buyer and seller do not, no service-only scope list gains it, then `make check` (or gofmt, vet, test)
- [x] 1.2 team-order: `ForceFailSaga` requires `admin` and `order.admin`; `isAdminOrUser` recognises only `order.admin`; `CreateOrder` untouched; verify handler tests for admin+order.admin (allowed), admin only (denied, order unchanged), order.admin only (denied), buyer, owner, service, anonymous, plus bare-`admin` `GetSagaState`/`UpdateOrderStatus` denied; tests fail without the change; `go vet ./... && go test ./...`
- [x] 1.3 team-gateway: carry `exp` and `sid` on the resolved principal; stream auth interceptor cancels the stream at `exp` and on denylist revocation (poll `STREAM_REVOCATION_CHECK_SECONDS`, default 5), returns `unauthenticated`, cancels upstream; setting in config, `.env.example`, README, `cmd/gateway/main.go`; verify unit tests with a fake stream conn: expiry cut, revocation cut, finish-before-expiry unaffected, watcher goroutine exits, upstream context cancelled; `make check`

## 2. E2E track (platform-e2e; `ar2_` step modules; owning repos' FEATURES.yaml)

- [x] 2.1 FEATURES.yaml acceptance lines (identity, order, gateway) with status `planned` and note `needs rebuild`; verify `make -C platform-e2e features-check`
- [x] 2.2 `security/ar2_order_admin.feature`: admin token carries `order.admin`, buyer/seller do not, admin-without-`order.admin` cannot force-fail or read a saga, admin can read a saga, buyer B cannot force-fail A's order; verify ruff, black and collection
- [x] 2.3 `security/ar2_stream_lifetime.feature`: expiry cut, revocation cut, finish-before-expiry unaffected; verify ruff, black and collection

## 3. Review and verify

- [x] 3.1 `auth-scope-reviewer` logic over the commits: every changed RPC gated
- [x] 3.2 Gate: `openspec validate authz-residuals-2 --strict`, `make -C platform-e2e features-check`
- [x] 3.3 After rebuild of identity, order and gateway: run the two features green (integrator)

## 4. Archive

- [x] 4.1 `openspec archive authz-residuals-2`; verify folded specs

## Evidence (2026-10-09)

- Gate after rebuilding identity, order, gateway, team-ai, analytics and frontend (images built from feat/ui-system):
  - parallel lane (`-n 4 -m "not destructive"`, deselecting test_ui_components): 710 passed and 4 xfailed, run twice
    (w2-par-1, w2-par-2). The 4 xfails are UI defects found by other tracks.
  - destructive lane: 75 passed, 3 failed (w2-destr). All 3 failures are in other changes' new tests
    (notification-delivery-hardening and ui-phase-cart-checkout) and are being fixed separately.
- Also `openspec validate --strict`, `spec_sync --strict` (e2e-ready), `features.py --strict` and repo_doctor.
- Reviews: auth-scope-reviewer and contract-boundary-reviewer found no blocking issue. Integrator fixes from review:
  - a stream that upstream ends with a clean EOF after the cut now still answers `unauthenticated`
    (team-gateway interceptor, new test);
  - the storefront admin order view checks `order.admin`, as the backend does.
- BREAKING: admin tokens issued before this change lack `order.admin`, so admins must sign in again.
- Follow-ups:
  - the SSE route `/api/events/live` is not cut at token expiry;
  - a token with no `sid` cannot be revoked mid-stream (same as unary calls);
  - `GetShipmentTracking` has no principal check (predates this change; confirm it is meant to be public).
