# Release (phase 8) and Operate (phase 9) on the local plane

Flow: `commit sha -> make -C deploy release-tag -> gitops-push -> ArgoCD sync (marketplace-staging) -> smoke-staging -> canary -> human approves prod tag -> (rollback path)`.

## Commands
| Step | Command |
|---|---|
| Plan the tag bump | `make -C deploy release-tag DRY_RUN=1` |
| Tag + push images + bump `envs/staging/values.yaml` | `make -C deploy release-tag [TAG=<sha>] [RELEASE_ENV=staging\|local\|prod]` (needs the images built locally; dirty tree refused unless `ALLOW_DIRTY=1`) |
| Publish desired state to local Gitea | `make -C deploy gitops-push` |
| Status | `make -C deploy status-staging` |
| Smoke | `make -C deploy smoke-staging` (frontend `http://marketplace-staging.localtest.me`, gateway `http://api.staging.localtest.me`) |
| Rollback | `scripts/rollback.sh <svc> [--env staging] [--abort] [--to <revision\|tag>]` |
| Alerts | `kubectl -n marketplace logs deploy/webhook-logger` |

## What staging-on-kind is
Edge only: `team-gateway` (canary Rollout) and `team-frontend` in namespace `marketplace-staging`, from `platform-gitops/argocd/apps/appset-staging-kind.yaml` + `envs/staging/kind/`. Backends are the shared local services in `marketplace`. Vault roles are bound to `marketplace`, so the staging gateway runs with `secrets.enabled=false`. Not a prod-like isolated environment; the point is to exercise tag bump, ArgoCD sync, canary analysis and rollback.

## Tagging rules
Image tag = short commit sha (12 hex) for every service, identical in one release. `local` keeps its content-hash tags from `make -C deploy images`.

## Runbooks
`docs/runbooks/<service>.md` (generated: `python3 docs/runbooks/gen_runbooks.py`), `rollback.md` (drill + Drill log), `alerts.md`. Agent: `.claude/agents/incident-triager.md` (read-only). Skill: `.claude/skills/release/SKILL.md`.
