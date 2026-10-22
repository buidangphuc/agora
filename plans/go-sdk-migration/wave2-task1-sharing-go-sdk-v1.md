# [W2-T1] sharing-go-sdk-v1

## Role
feature-run worker (change `openspec/changes/sharing-go-sdk-v1`)

## Objective
Implement OpenSpec change `sharing-go-sdk-v1` test-first per its tasks.md: migrate team-sharing to go-sdk v1.0.2 (tag exists), ADR-0016 layout, shared make/lint, delete dependency enable flags and the runtime in-memory fallback, gitops ENV overlays and probes.livenessService; context design.md Clarifications.

## Write-set (EXCLUSIVE)
- team-sharing/**
- docker-compose.services.yaml
- platform-gitops/envs/services/team-sharing.yaml
- platform-gitops/argocd/apps/team-sharing.yaml
- platform-gitops/envs/staging/services/team-sharing.yaml
- platform-gitops/envs/prod/services/team-sharing.yaml
- platform-e2e/tests/e2e/**
- openspec/changes/sharing-go-sdk-v1/**

## Verify
- cd team-sharing && go build ./... && go vet ./... && go test ./... -count=1
- docker build -t team-sharingocal team-sharing
- the team-sharing e2e feature files named in tasks.md (e2e.sh against mainx)
- openspec validate sharing-go-sdk-v1 --strict
- make -C platform-e2e features-check
