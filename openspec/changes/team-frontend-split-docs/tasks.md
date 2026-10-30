## 1. Spec and manifest references

- [x] 1.1 Rewrite stale `team-frontend` paths in `openspec/specs/**/spec.md` to the seller or admin app; verify no path under `team-frontend*/` named in specs or manifests is missing, except the pre-existing `team-frontend/e2e/referrer-hygiene.spec.ts`
- [x] 1.2 Move seller and admin entries into `team-frontend-seller/FEATURES.yaml` and `team-frontend-admin/FEATURES.yaml`; verify `features.py --strict` reports the same totals (1449 automated) and every `covered_by` resolves

## 2. Docs

- [x] 2.1 Update the three app READMEs, `team-gateway/README.md` and `platform-e2e/README.md`; verify `repo_doctor.py` finds no stale env var
- [x] 2.2 Update `AGENTS.md` and `docs/agents/project.md`
- [x] 2.3 Update ADR-0006 status, what was built and open risks

## 3. Verification

- [x] 3.1 `scripts/asdlc/ci_local.sh specs features repo` with `BASE=origin/feat/team-frontend-f`
- [x] 3.2 `openspec validate team-frontend-split-docs --strict`
