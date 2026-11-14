# [W3-T1] domain-bundle-integrity

## Role
feature-run worker (change `openspec/changes/domain-bundle-integrity`)

## Objective
Implement OpenSpec change `domain-bundle-integrity` test-first per its tasks.md; context in plans/seller-centre/00-spec.md and the decisions in plans/seller-centre/LOCK.md.
- `BundleService.Create` validates 2..10 listings, no duplicates, owned and existing (one answer for foreign/unknown → PERMISSION_DENIED), no variants, price in (0, sum) via one batched read; legacy bad bundles still readable.
- depends_on: none (wave 3 because team-domain slices are serialised: `rpc/listing.go`, `rpc/errors.go`, store, FEATURES.yaml)
- base: `asdlc/seller-centre` after wave 2

## Write-set (EXCLUSIVE)
- team-domain/internal/listing/bundle.go
- team-domain/internal/listing/bundle_test.go
- team-domain/internal/rpc/errors.go
- team-domain/internal/rpc/errors_test.go
- team-domain/internal/rpc/bundle.go
- team-domain/internal/rpc/bundle_test.go
- team-domain/internal/rpc/listing.go
- team-domain/internal/store/**
- team-domain/internal/app/**
- team-domain/FEATURES.yaml
- team-domain/README.md
- platform-e2e/tests/e2e/features/seller/bundle_integrity.feature
- platform-e2e/tests/e2e/step_definitions/bundle_integrity_steps.py
- platform-e2e/tests/e2e/step_definitions/test_bundle_integrity.py
- platform-e2e/test-data/bundle_integrity/**
- platform-e2e/src/api/services/listing_service.py
- widened (P1):
  - docs/asdlc/ci.md — doc updated by docs-sync (tdd row: red_first drops remote BSR plugins without a local binary, scripts/asdlc/red_first.sh:173)

## Verify
- `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1 /Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py go team-domain --vet`
- e2e: bundle_integrity feature against env seller-centre (team-gateway, team-domain, team-identity)
- `/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python scripts/asdlc/gate_digest.py spec-sync . domain-bundle-integrity` and `openspec validate domain-bundle-integrity --strict`
