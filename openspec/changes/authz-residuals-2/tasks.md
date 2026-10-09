## 1. Code track (one agent per repo, worktree each)

- [ ] 1.1 team-identity: add `order.admin` to `roleScopes[RoleAdmin]` only; verify authz tests: admin has `admin` and `order.admin`, buyer and seller do not, no service-only scope list gains it, then `make check` (or gofmt, vet, test)
- [ ] 1.2 team-order: `ForceFailSaga` requires `admin` and `order.admin`; `isAdminOrUser` recognises only `order.admin`; `CreateOrder` untouched; verify handler tests for admin+order.admin (allowed), admin only (denied, order unchanged), order.admin only (denied), buyer, owner, service, anonymous, plus bare-`admin` `GetSagaState`/`UpdateOrderStatus` denied; tests fail without the change; `go vet ./... && go test ./...`
- [ ] 1.3 team-gateway: carry `exp` and `sid` on the resolved principal; stream auth interceptor cancels the stream at `exp` and on denylist revocation (poll `STREAM_REVOCATION_CHECK_SECONDS`, default 5), returns `unauthenticated`, cancels upstream; setting in config, `.env.example`, README, `cmd/gateway/main.go`; verify unit tests with a fake stream conn: expiry cut, revocation cut, finish-before-expiry unaffected, watcher goroutine exits, upstream context cancelled; `make check`

## 2. E2E track (platform-e2e; `ar2_` step modules; owning repos' FEATURES.yaml)

- [ ] 2.1 FEATURES.yaml acceptance lines (identity, order, gateway) with status `planned` and note `needs rebuild`; verify `make -C platform-e2e features-check`
- [ ] 2.2 `security/ar2_order_admin.feature`: admin token carries `order.admin`, buyer/seller do not, admin-without-`order.admin` cannot force-fail or read a saga, admin can read a saga, buyer B cannot force-fail A's order; verify ruff, black and collection
- [ ] 2.3 `security/ar2_stream_lifetime.feature`: expiry cut, revocation cut, finish-before-expiry unaffected; verify ruff, black and collection

## 3. Review and verify

- [ ] 3.1 `auth-scope-reviewer` logic over the commits: every changed RPC gated
- [ ] 3.2 Gate: `openspec validate authz-residuals-2 --strict`, `make -C platform-e2e features-check`
- [ ] 3.3 After rebuild of identity, order and gateway: run the two features green (integrator)

## 4. Archive

- [ ] 4.1 `openspec archive authz-residuals-2`; verify folded specs
