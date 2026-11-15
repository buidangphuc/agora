# [W1-T3] promotion-seller-self-service

## Role
feature-run worker (change `openspec/changes/promotion-seller-self-service`)

## Objective
Implement OpenSpec change `promotion-seller-self-service` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- team-promotion: `ListMyCampaigns`, `CancelMyCampaign` (SCHEDULED only, owner-checked, no audit), `ListMyShopVouchers`, `DeactivateMyShopVoucher`; migration `0009_campaign_seller` + owner recorded on create; `backfill-campaign-sellers` subcommand.
- team-gateway: forwarders for the four RPCs in `internal/edge/promotion.go`; `policy.go` untouched.
- depends_on: seller-centre-contract
- base: `asdlc/seller-centre` after wave 0

## Write-set (EXCLUSIVE)
- team-promotion/internal/**
- team-promotion/cmd/**
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion_seller_test.go
- team-gateway/README.md
- platform-e2e/tests/e2e/features/promo/promotion_seller_self_service.feature
- platform-e2e/tests/e2e/step_definitions/promotion_seller_self_service_steps.py
- platform-e2e/tests/e2e/step_definitions/test_promotion_seller_self_service.py
- platform-e2e/src/api/services/promotion_service.py
- openspec/changes/promotion-seller-self-service/specs/promotion-seller-self-service/spec.md (widened at build: P1, VERIFIED BY citations aligned to the real test names (no requirement or scenario wording change))

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-promotion --vet`
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway --vet` (stubs: `buf generate --template buf.gen.local.yaml` first)
- e2e: promotion_seller_self_service feature + existing promotion_admin_campaigns / promotion_admin_vouchers regression against env seller-centre (team-gateway, team-promotion, team-domain, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . promotion-seller-self-service` and `openspec validate promotion-seller-self-service --strict`
