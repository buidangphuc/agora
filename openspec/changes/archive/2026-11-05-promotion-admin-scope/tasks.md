## 1. Code track (one repo per task, test-first)

- [x] 1.1 team-identity: add `promotion.admin` to `roleScopes[RoleAdmin]` only and list it in the `scopes.go` doc comment; verify `internal/authz` tests: admin has `admin`, `order.admin` and `promotion.admin`; buyer and seller do not; it is not a service-only scope; tests red before the change; `make lint`, `go vet ./... && go test ./...`
- [x] 1.2 team-promotion: `requireAdmin` needs `admin` and `promotion.admin`; `isPromotionAdmin` (`promotion.admin`) drives the shop-voucher audit flag, the flash-sale admin path and the ad-campaign `IsAdmin`; `requireSeller` accepts `promotion.admin` or `listing.write`; verify `internal/rpc` tests per RPC (both scopes allowed; `admin` only denied or seller path; `promotion.admin` only denied on admin-only RPCs; seller, buyer, anonymous refused; no audit on refused or seller paths); tests red before the change; `make lint`, `go vet ./... && go test ./...`
- [x] 1.3 team-gateway: map the five promotion admin procedures to `promotion.admin` in `adminProcedures`; verify `internal/edge` policy tests (token with `admin` only refused with 403 on the five, `promotion.admin` allowed, anonymous 401, other admin procedures unchanged); tests red before the change; `make lint`, `go vet ./... && go test ./...`
- [x] 1.4 Docs: `AGENTS.md` rule 6 and `.claude/agents/auth-scope-reviewer.md` name `promotion.admin` next to `order.admin`; verify grep for `promotion.admin` in both files

## 2. E2E track (platform-e2e; owning repos' FEATURES.yaml)

- [x] 2.1 [e2e] FEATURES.yaml acceptance entries in team-identity (capability: auth), team-gateway (capability: edge-route-policy) and team-promotion (capability: promotion-access-control) with status `planned` and note `needs rebuild`; verify `make -C platform-e2e features-check`
- [x] 2.2 [e2e] `security/promotion_admin_scope.feature` with steps for every scenario of the auth, edge-route-policy and promotion-access-control deltas, dev tokens signed with the local identity key for the admin-only and promotion.admin-only callers; verify ruff, black and pytest collection (capability: auth, edge-route-policy, promotion-access-control)
- [x] 2.3 [e2e] After rebuilding team-identity, team-promotion and team-gateway: run the feature green and flip the FEATURES entries to `automated` (capability: auth, edge-route-policy, promotion-access-control)

## 3. Review and verify

- [x] 3.1 `auth-scope-reviewer` over the diff: every changed RPC gated, admin branches audited
- [x] 3.2 Gate: `openspec validate promotion-admin-scope --strict`, `scripts/asdlc/spec_lint.py promotion-admin-scope`, `make -C platform-e2e features-check`
