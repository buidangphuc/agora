# [W2-T1] domain-storefront-customisation

## Role
feature-run worker (change `openspec/changes/domain-storefront-customisation`)

## Objective
Implement OpenSpec change `domain-storefront-customisation` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `StorefrontService.Upsert` validates tagline (≤ 120 runes, trimmed, no control chars), theme (default/dark/festive), banner (storage public base, ≤ 2048, legacy exemption) and featured ids (≤ 6, no dupes, owned, one batched listing read); errors map to INVALID_ARGUMENT; owner forcing and scope check unchanged.
- depends_on: none (wave 2 because team-domain slices are serialised: shared store port, `rpc/errors.go`, FEATURES.yaml)
- base: `asdlc/seller-centre` after wave 1

## Write-set (EXCLUSIVE)
- team-domain/internal/listing/storefront.go
- team-domain/internal/listing/storefront_test.go
- team-domain/internal/listing/listing.go
- team-domain/internal/rpc/errors.go
- team-domain/internal/rpc/errors_test.go
- team-domain/internal/rpc/storefront.go
- team-domain/internal/rpc/storefront_test.go
- team-domain/internal/objects/**
- team-domain/internal/store/**
- team-domain/internal/app/**
- team-domain/cmd/server/**
- team-domain/FEATURES.yaml
- team-domain/README.md
- platform-e2e/tests/e2e/features/domain/storefront_customisation.feature
- platform-e2e/tests/e2e/step_definitions/storefront_customisation_steps.py
- platform-e2e/tests/e2e/step_definitions/test_storefront_customisation.py
- platform-e2e/src/api/services/listing_service.py
- platform-e2e/src/constants/gateway_endpoints.py (widened at build: P1, e2e support constant (LISTING_GET_IMAGE_UPLOAD_URL) used by this change's listing_service.py e2e client)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-domain --vet`
- e2e: storefront_customisation feature against env seller-centre (team-gateway, team-domain, team-identity, minio)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . domain-storefront-customisation` and `openspec validate domain-storefront-customisation --strict`
