# [W1-T1] gateway-principal-log-fingerprint

## Role
feature-run worker (change `openspec/changes/gateway-principal-log-fingerprint`)

## Objective
Implement OpenSpec change `gateway-principal-log-fingerprint` test-first per its tasks.md; context in plans/p10-backend-gaps/00-spec.md (Locked + Clarifications) and the change's design.md.
- Replace raw `principal` log keys in team-gateway with `principal_fp` (HMAC via kafkax.NewFingerprinter, LOG_KEY_FP_SECRET) and `principal_kind`; forbid the `principal` key in the shared go-sdk lint.

## Write-set (EXCLUSIVE)
- team-gateway/internal/config/config.go
- team-gateway/internal/config/config_test.go
- team-gateway/internal/app/run.go
- team-gateway/internal/edge/interceptors.go
- team-gateway/internal/edge/interceptors_test.go
- team-gateway/internal/edge/http.go
- team-gateway/internal/edge/forward.go
- team-gateway/internal/edge/forward_test.go
- team-gateway/internal/edge/principal_log.go
- team-gateway/internal/edge/principal_log_test.go
- team-gateway/internal/edge/edge_logging_test.go
- team-gateway/internal/edge/edge_residuals_test.go
- team-gateway/internal/edge/stream_lifetime.go
- team-gateway/internal/edge/realtime.go
- team-gateway/.env.example
- team-gateway/README.md
- team-gateway/FEATURES.yaml
- docker-compose.services.yaml
- platform-gitops/platform/vault-config/vault-config.yaml
- platform-gitops/envs/services/team-gateway.yaml
- platform-gitops/argocd/apps/team-gateway.yaml
- platform-core/packages/go-sdk/.golangci.yml
- platform-e2e/tests/e2e/features/security/gateway_principal_log_fingerprint.feature
- platform-e2e/tests/e2e/step_definitions/gateway_principal_log_fingerprint_steps.py
- platform-e2e/tests/e2e/step_definitions/test_gateway_principal_log_fingerprint.py
- openspec/changes/gateway-principal-log-fingerprint/**
- team-gateway/internal/app/fingerprint_boot_test.go (widened at build: task 1.2)
- team-gateway/internal/edge/log_audit_test.go (widened at build: task 1.4)
- scripts/asdlc/tests/test_gateway_log_key_fp_secret_declared.py (widened at build: task 1.6)

## Read-only dependencies
- team-identity sdkconfig.LogKeyFP embedding (pattern)
- platform-e2e/tests/e2e/support/oic_inv_stack.py container_logs helper
- plans/p10-backend-gaps/00-spec.md, LOCK.md

## Verify
PY=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/python; run every check through the gate-digest skill (scripts/asdlc/gate_digest.py).
- (cd team-gateway && go test ./... -race)
- make -C team-gateway check
- golangci-lint across all Go repos with the updated platform-core/packages/go-sdk/.golangci.yml (sequentially): zero findings
- bash /Users/phuc.buidang/Library/Caches/ai-first-runs/p10-build/e2e.sh <slice-worktree> platform-e2e/tests/e2e/step_definitions/test_gateway_principal_log_fingerprint.py
- $PY platform-e2e/scripts/spec_sync.py gateway-principal-log-fingerprint --strict
