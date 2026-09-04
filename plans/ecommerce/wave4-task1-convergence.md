# [W4-T1] Convergence — full suite, drift check, release handoff

## Role
SRE   # integration + release readiness

## Objective
All waves merged; full stack green end-to-end; no cross-repo drift; release delta documented.

## Write-set (EXCLUSIVE)
- plan-ecommerce/99-release.md   (create — release delta)
- (residual wiring only if a STOP report surfaced a missed registration; otherwise none)

## Verify
```bash
# 1. Per-repo Go gates (in Docker) for every changed service
for r in team-promotion team-order team-search team-engagement team-notification team-gateway; do
  (cd $r && gofmt -l . && go vet ./... && go test ./...) || echo "FAIL $r"; done
# 2. Contract gates (in Docker, from platform-core)
buf lint && buf breaking
# 3. Frontend
(cd team-frontend && npm run typecheck && npm run build)
# 4. Drift
python scripts/repo_doctor.py
# 5. Full stack + e2e
docker compose -p platform-core up -d
make -C platform-e2e collect && make -C platform-e2e test
# 6. Plan audit
python scripts/validate_plan.py plan-ecommerce/
```

## Acceptance criteria
- [ ] All per-repo Go gates green; buf lint/breaking green; frontend builds.
- [ ] repo_doctor: no compose/port-table/proto-regen drift.
- [ ] platform-e2e full suite green; all 4 features' FEATURES.yaml = automated.
- [ ] 99-release.md written (delta only).

## Review (gate)
Self + repo-doctor skill output attached.

## Out of scope
- No new features. No refactors. Only wiring fixes explicitly surfaced by STOP reports.
