# [W2-T2] promotion-ads-campaign-mgmt

## Role
feature-run worker (change `openspec/changes/promotion-ads-campaign-mgmt`)

## Objective
Implement OpenSpec change `promotion-ads-campaign-mgmt` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-promotion `ListMyAdCampaigns` (cursor `created_at DESC, id DESC`, clamp), `PauseAdCampaign` / `ResumeAdCampaign` as conditional UPDATEs scoped by `id AND seller_id`, NOT_FOUND for foreign/unknown, FAILED_PRECONDITION for bad state.
- team-gateway forwards the three RPCs on `SponsoredForwarder`.
- depends_on: promotion-ads-serving
- base: `asdlc/monetization` after wave 1
- hot file: team-promotion sponsored files and team-gateway/internal/edge/promotion.go (after promotion-ads-serving, before promotion-plan-billing)

## Write-set (EXCLUSIVE)
- team-promotion/internal/**
- team-promotion/cmd/**
- team-promotion/go.mod
- team-promotion/go.sum
- team-promotion/README.md
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion_ad_campaigns_test.go
- platform-e2e/tests/e2e/features/promotion/ad_campaign_management.feature
- platform-e2e/tests/e2e/step_definitions/ad_campaign_management_steps.py
- platform-e2e/tests/e2e/support/ad_campaign_management_support.py
- platform-e2e/tests/e2e/step_definitions/test_ad_campaign_management.py

### widened (P1)
- platform-e2e/tests/e2e/step_definitions/test_ad_campaign_management_steps.py (e2e step file of this change, named differently from the packet)
- team-gateway/README.md (doc updated by docs-sync: ad campaign RPCs now forwarded with an edge auth check)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-promotion` (equivalent: `cd team-promotion && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway` (equivalent: `cd team-gateway && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-promotion && make check` and `cd team-gateway && make check`
- e2e: promotion/ad_campaign_management.feature against env monetization (team-promotion, team-gateway, team-domain)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . promotion-ads-campaign-mgmt`
