# [W1-T4] promotion-ads-serving

## Role
feature-run worker (change `openspec/changes/promotion-ads-serving`)

## Objective
Implement OpenSpec change `promotion-ads-serving` test-first per its tasks.md; context in plans/monetization/00-spec.md (decisions in LOCK.md, defaults in brackets apply).
- team-promotion: `MIN_AD_BID`, campaign columns (spent, impressions, clicks), `ad_impressions` table, `PAUSED_BUDGET`, `ListSponsoredSlots` with placement and tokens, `RecordAdClick` with compare-and-set charge, 30-day impression purge in the sweeper.
- team-gateway `SponsoredForwarder.RecordAdClick`, anonymous allowed. Migration number: next free after 0008.
- depends_on: monetization-contract
- base: `asdlc/monetization` after wave 0
- hot file: team-promotion migrations, internal/sponsored/sponsored.go, internal/store/sponsored.go and team-gateway/internal/edge/promotion.go shared with promotion-ads-campaign-mgmt (wave 2) and promotion-plan-billing (wave 3)

## Write-set (EXCLUSIVE)
- team-promotion/internal/**
- team-promotion/cmd/**
- team-promotion/go.mod
- team-promotion/go.sum
- team-promotion/README.md
- team-promotion/.env.example
- team-promotion/FEATURES.yaml
- team-gateway/internal/edge/promotion.go
- team-gateway/internal/edge/promotion_sponsored_click_test.go
- platform-e2e/tests/e2e/features/ads/sponsored_serving.feature
- platform-e2e/tests/e2e/features/ads/sponsored_click.feature
- platform-e2e/tests/e2e/step_definitions/sponsored_ads_steps.py
- platform-e2e/tests/e2e/support/sponsored_ads_support.py
- platform-e2e/tests/e2e/step_definitions/test_sponsored_serving.py
- platform-e2e/tests/e2e/step_definitions/test_sponsored_click.py
### widened (P1)
- openspec/changes/promotion-ads-serving/tasks.md (task ticks in this change's own OpenSpec folder)
- platform-e2e/pyproject.toml (pytest marker registration: ads; user decision 2026-11-15)
- platform-e2e/tests/e2e/step_definitions/test_sponsored_ads_steps.py (e2e step file of this change binding both ads features, named differently from the packet)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-promotion` (equivalent: `cd team-promotion && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-gateway` (equivalent: `cd team-gateway && DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 go test ./...`)
- `cd team-promotion && make check` and `cd team-gateway && make check` (stubs: `buf generate --template buf.gen.local.yaml`)
- e2e: ads/sponsored_serving.feature, ads/sponsored_click.feature against env monetization (team-promotion, team-gateway, team-domain)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . promotion-ads-serving`
