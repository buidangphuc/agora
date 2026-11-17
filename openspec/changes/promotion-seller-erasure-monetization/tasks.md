## 1. Code track (team-promotion)

- [ ] 1.1 Store tests first in `internal/store/erasure_test.go` (Docker, `GO_SDK_REQUIRE_DOCKER=1`): subscription ended, not due for renewal, ads ended, ended campaign not served or charged, idempotency over subscription and campaign rows, rollback with a failure after the subscription update; verify red (scenarios: Erasure ends an auto-renewing paid plan now, The renewal sweep never charges an erased seller, A seller without a paid plan is unchanged, Redelivery leaves the ended subscription unchanged, A store error leaves the plan running for retry, All non-ended campaigns of the seller end, An ended campaign is neither served nor charged, Redelivery leaves the ended campaigns unchanged)
- [ ] 1.2 `internal/store/erasure.go`: in the existing transaction, end the seller's subscription (plan FREE, `cancelled`, no auto-renew, no pending plan) when it is ACTIVE, auto-renewing or has a pending plan, and set ACTIVE/PAUSED/PAUSED_BUDGET campaigns of the seller to ENDED (same scenarios as 1.1)
- [ ] 1.3 README Events table row for `UserErased` describes the subscription and campaign effects
- [ ] 1.4 `go vet ./... && go test ./...` green in team-promotion

## 2. e2e track (platform-e2e)

- [ ] 2.1 Feature `tests/e2e/features/promotion/seller_erasure_monetization.feature` "An erased seller's plan is never renewed" (scenario: An erased seller's plan is never renewed on the real stack)
- [ ] 2.2 "An erased seller's ads stop serving" (scenario: An erased seller's ads stop serving on the real stack)
- [ ] 2.3 Register the scenarios in team-promotion FEATURES.yaml

## 3. Verification

- [ ] 3.1 `openspec validate promotion-seller-erasure-monetization --strict`, spec_lint, features check
- [ ] 3.2 e2e green on a feature env (`agora_env.py up` + `e2e`)
- [ ] 3.3 `scripts/asdlc/ci_local.sh` green
