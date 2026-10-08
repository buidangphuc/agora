## 1. Code track (one agent per repo, worktree each; done, commits on feat/ui-system)

- [x] 1.1 team-gateway: unroute internal RPCs (01a14d7), edge admin policy (e2d2210), error sanitiser (ea3c521), JWT exp/sub (83ffa0f), request-id and Idempotency-Key validation (421ca14); verify `go vet ./... && go test ./...`
- [x] 1.2 team-audit: service-only WriteAuditEvent (ea20aa4), generic INTERNAL (4d4b5ae); verify unit tests
- [x] 1.3 team-promotion: saga gates, preview namespace, subscription and ad scopes and ownership, entitlements, ad caps, upstream timeout, generic errors (124d378); verify `make check`
- [x] 1.4 team-order: least-privilege upstream principals incl. `promotion.reserve` (a20eb24), error hygiene (97fd4e7), self-purchase (4f39e28), durable-storage guard (8088ff1); verify unit tests incl. the upstream metadata wire test
- [x] 1.5 team-engagement: bounded order lookup (2d34d22), CreateDispute verification (cd15cd7), GetDispute access (1c547c0), shop-reply ownership (5894578), admin ResolveDispute and generic errors (78112fa); verify `make check`
- [x] 1.6 team-domain: draft visibility and published default (b28e326), strict config (65cbe77), generic errors (2c6b1cb); verify unit tests
- [x] 1.7 team-search: published-only and draft policy (692174a), saved-search user and durable storage guard (630130a); verify `make check`
- [x] 1.8 team-payment: MOCK_PAYMENTS gate and boot guard, generic errors (de7ce3c); verify unit tests
- [x] 1.9 team-identity: scope grants and drift test (f2cf79c), login recording (72dc5b1), generic errors (e112a4f), reset log (cbda781), dev key guard (1e3bfe8); verify `make check`
- [x] 1.10 team-ai: scope gates (a3d7099), bearer fallback and Recommend binding (e8bfe03), rate limiter (7eb7550), quota upsert fix (2c30b6f); verify `make test`
- [x] 1.11 team-chat, team-notification, team-sharing, team-verification, team-referral: generic INTERNAL errors (6bd4bec, 88c7caf, bd0bb88, 85c2380, cf17edb); verify unit tests
- [x] 1.12 platform-gitops: ENV in staging/prod overlays (b44ba6b); local kind and compose MOCK_PAYMENTS (17429ad); verify `helm template`, `docker compose config`
- [x] 1.13 team-frontend: drop dead reserveStock/releaseStock (91c1869); verify `npm run check`

## 2. E2E track (platform-e2e; owning repos' FEATURES.yaml)

- [x] 2.1 Add a FEATURES.yaml entry per scenario in the owning repo (gateway, audit, promotion, order, engagement, domain, search, payment, ai, identity), each with `covered_by` pointing at a real `.feature` scenario; verify `make -C platform-e2e features-check`
- [x] 2.2 Edge-route-policy and audit scenarios (`security/edge_route_policy.feature`), including the token-claim cases signed with the local dev key and the stopped-upstream case in the destructive lane; verify they pass against the stack
- [x] 2.3 Promotion, order and payment scenarios (`security/promotion_access.feature`, `order/self_purchase.feature`); verify they pass and that the existing voucher checkout scenarios stay green
- [x] 2.4 Dispute and Q&A scenarios (`security/dispute_access.feature`); verify they pass
- [x] 2.5 Listing and search visibility scenarios (`security/listing_visibility.feature`); verify they pass
- [x] 2.6 AI and auth scenarios (`security/ai_access.feature`, `auth/login_history.feature`); verify they pass
- [x] 2.7 Boot-guard scenarios running the real images as black boxes (`ops/boot_guards.feature`, destructive lane); verify they pass
- [x] 2.8 Rewrite the steps broken by the new contract: the audit write scenario in `new_services.feature` (now 501) and the dispute defendant id in `group_b_steps.py`; verify they pass

## 3. Review and verify

- [x] 3.1 Run `auth-scope-reviewer` and `contract-boundary-reviewer` over `b7d5d3f..HEAD`; verify no blocking finding remains (fix each in its own commit)
- [x] 3.2 Gate: `openspec validate port-security-hardening --strict`, `make -C platform-e2e features-check`, `make -C platform-e2e spec-check CHANGE=port-security-hardening`, `repo_doctor`
- [x] 3.3 Full e2e suite green twice in the parallel lane (`-n 4`) plus the destructive lane; every new flake root-caused

## Evidence (2026-10-08)

- e2e: 41/41 scenarios automated (spec_sync --strict). Full suite: 284 passed and 284 passed (`-n 4`, not destructive), then 7 passed in the destructive serial lane.
- Defects the e2e track found, each fixed in its own commit:
  - the gateway leaked LB/resolver text on deadline_exceeded (0fef841);
  - the team-search boot guard ran after connecting to OpenSearch (a3730c1);
  - the team-identity boot guard ran after Postgres (4218558).
- Spec corrected where the code was right: `GetEntitlements` may return a seller's own entitlements, and an unreachable upstream may answer 504.
- Reviews:
  - contract-boundary: no blocking findings. Follow-ups 5d7a88b (dev private key out of testdata) and 9bfef12 (AGENTS.md edge policy).
  - auth-scope: no blocking findings. Follow-ups 10a989d (unknown principal type is anonymous), 48e3b4c (`identity.read` pinned) and 85cfbfc (`all` wildcard dropped).
- Open items handed to the next waves:
  - `ai:use` enforcement (`AI_USE_SCOPE_REQUIRED`, a product decision);
  - preview reservation TTL sweeper and rate limit;
  - team-order deduplication on Idempotency-Key;
  - mTLS or service tokens between services (AGENTS §9);
  - `JWT_SECRET` residue in the team-frontend gitops values.

## 4. Archive

- [ ] 4.1 `openspec archive port-security-hardening` after the gate is green; verify the deltas are folded into `openspec/specs/`
