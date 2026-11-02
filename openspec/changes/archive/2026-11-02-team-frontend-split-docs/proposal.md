## Why

P10 split the storefront into three apps (buyer `team-frontend`, `team-frontend-seller`, `team-frontend-admin`). The seller and admin code moved, but `openspec/specs` still pointed `VERIFIED BY` lines at `team-frontend` paths that no longer exist, all seller and admin entries were still in `team-frontend/FEATURES.yaml`, and the READMEs, `AGENTS.md`, `docs/agents/project.md` and ADR-0006 did not describe what was built or what is still open. Plan step G closes that.

## What Changes

- `openspec/specs/**/spec.md`: `VERIFIED BY` paths and requirement text that named `team-frontend/src/app/(shop)/{seller,admin}`, `src/features/{seller,admin}`, the moved listing tests and the admin gateway tests now name the file in `team-frontend-seller` or `team-frontend-admin`.
- `FEATURES.yaml`: new manifests `team-frontend-seller/FEATURES.yaml` (96 entries) and `team-frontend-admin/FEATURES.yaml` (211 entries), moved out of `team-frontend/FEATURES.yaml` (671 left). Moved entries list their own app in `services`. Entries, ids, titles and `covered_by` are otherwise unchanged.
- READMEs: seller (TRUSTED_PROXIES gap restated with both gitops files, `NEXT_PUBLIC_BUYER_APP_URL` build-time note, manifest), admin (env vars, compose run, CI, spec and verification, TRUSTED_PROXIES, OTEL and SSO gaps), buyer (counts, compose note, ADR link), `team-gateway` (compose `TRUSTED_PROXIES`), `platform-e2e` (`ADMIN_BASE_URL`).
- `AGENTS.md` (recipe step 4 and the frontend convention with the vendor recipe), `docs/agents/project.md` (feedback loops, e2e base URLs, FEATURES ownership, the two ADR-0006 files).
- `fe-platform/docs/ADR/0006-three-apps-on-the-platform.md`: status, what was built, open risks.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`. The edits to `openspec/specs` change file references only, not requirements or scenarios.

## Impact

- Docs, OpenSpec specs and feature manifests only; no code, config or test changes.
- `features.py`, `spec_sync.py` and `cross_change_check.py` discover every `*/FEATURES.yaml`, so the split needs no gate change.

## Non-goals

Wiring the seller and admin apps into ArgoCD, fixing the gitops gateway `TRUSTED_PROXIES`, setting `ADMIN_BASE_URL` in the e2e runner, SSO (G8), the never-written `team-frontend/e2e/referrer-hygiene.spec.ts` reference in `openspec/specs/referrer-hygiene`.

## Non-functional requirements

`scripts/asdlc/ci_local.sh specs features repo` green with `BASE=origin/feat/team-frontend-f`; `openspec validate team-frontend-split-docs --strict`.
