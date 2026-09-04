# Plan Index — 12 features (4-PM roundtable)

| Wave | Task | Slug | Owner repo (writes) | Verify | Status |
|---|---|---|---|---|---|
| 0 | W0 | proto+scaffold-contracts | platform-core (8 domain protos + 4 new packages); regen FE stubs | buf lint clean | **done** |
| 1 | F1 | follow-seller | team-engagement/** | make proto && make test | **done** (go test green) |
| 1 | F2 | rich-chat | team-chat/** | buf generate && go test ./... | **done** (green) |
| 1 | F3 | notif-prefs | team-notification/** | (make proto||buf generate) && go test ./... | **done** (6 tests green) |
| 1 | F4 | seller-analytics | team-analytics/** | make proto && make test | **done** (green; warehouse schema adapted) |
| 1 | F5 | storefront | team-domain/** | make proto && make test | **done** (green) |
| 1 | F6 | seller-wallet | team-payment/** | buf generate && go test ./... | **done** (green, mock) |
| 1 | F7 | subscription-tiers | team-promotion/** | make proto && make test | **done** (green, mock) |
| 1 | F8 | ai-review-summary | team-ai/** | make test | **done** (438 tests green) |
| 2 | F9 | team-referral | team-referral/** (new) | go test ./... | **done** (8 tests green) |
| 2 | F10 | team-sharing | team-sharing/** (new) | go test ./... | **done** (green) |
| 2 | F11 | team-audit | team-audit/** (new) | go test ./... | **done** (5 tests green) |
| 2 | F12 | team-verification | team-verification/** (new) | go test ./... | **done** (10 tests green) |
| 3 | I1 | gateway-forwarders | team-gateway/** | make check | **done** (12 forwarders, make check green) |
| 3 | I2 | infra (compose+gitops) | docker-compose.services.yaml, platform-gitops/**, AGENTS.md | compose config parse | **done** (4 svcs wired, ports 50062/64/65/66) |
| 3 | I3 | thin frontend UI | team-frontend/** | tsc + vitest | **done** (8 Tier-A UIs, tsc clean) |

**Final verification:** gateway builds (12 forwarders + 6 new upstream clients registered: AnalyticsQuery, Subscription, Referral, Sharing, Audit, Verification); 4 new services build; frontend tsc clean; `docker compose config` parses. Frontend vitest: only the pre-existing `src/lib/track.test.ts` (4) fail. BSR rate-limit worked around with local protoc plugins where needed. See 99-release.md for live-stack handoff.

**Wave 0/1 notes:** all 8 Tier-A services build clean (`go build ./...` OK; team-ai import OK). Each agent verified `go test`/`make test` green in isolation. Transient buf.build BSR rate-limit on repeated `make proto` — harmless (stubs generated in Wave 0). Wave 2 = 4 new service repos, scaffold by mirroring team-notification. Wave 3 = integration.
