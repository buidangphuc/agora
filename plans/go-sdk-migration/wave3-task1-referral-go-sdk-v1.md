# [W3-T1] referral-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/referral-go-sdk-v1`)

## Objective
Implement OpenSpec change `referral-go-sdk-v1` test-first per its tasks.md: migrate team-referral to go-sdk v1.0.2 (tag exists), ADR-0016 layout, shared make/lint, delete dependency enable flags and the runtime in-memory fallback, gitops ENV overlays and probes.livenessService; context design.md Clarifications.

## Write-set (EXCLUSIVE)
- team-referral/**
- docker-compose.services.yaml
- platform-gitops/envs/services/team-referral.yaml
- platform-gitops/argocd/apps/team-referral.yaml
- platform-gitops/envs/staging/services/team-referral.yaml
- platform-gitops/envs/prod/services/team-referral.yaml
- platform-e2e/tests/e2e/**
- openspec/changes/referral-go-sdk-v1/**
- team-referral/Makefile (widened at build: task 1.1)
- team-referral/.env.example (widened at build: task 1.1)
- docker-compose.services.yaml (widened at build: task 1.1)
- platform-gitops/envs/services/team-referral.yaml (widened at build: task 1.1)
- platform-gitops/argocd/apps/team-referral.yaml (widened at build: task 1.1)
- platform-gitops/envs/staging/services/team-referral.yaml (widened at build: task 1.1)
- platform-gitops/envs/prod/services/team-referral.yaml (widened at build: task 1.1)

## Verify
- cd team-referral && go build ./... && go vet ./... && go test ./... -count=1
- docker build -t team-referralocal team-referral
- the team-referral e2e feature files named in tasks.md (e2e.sh against mainx)
- openspec validate referral-go-sdk-v1 --strict
- make -C platform-e2e features-check
