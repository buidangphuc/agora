# OpenSpec — Archive status (unarchived changes)

_Generated 2026-10-11. Rule in force: **do not force-archive incomplete changes.**_

**1 open change, 86 archived** (`openspec/changes/archive/`). The 15 changes listed in the 2026-09-04
version of this file have all been archived since. Open-change count vs WIP limit: 1 of 3 (profile `solo`,
`asdlc.config.yaml`).

| Change | Tasks | Primary blocker(s) |
|---|---|---|
| add-ga4-gtm-datalayer | 2/8 | Implementation not started: team-frontend dataLayer module + call-site migration, team-gateway collector aliases, team-analytics migration/view, platform-recsys weights, E2E BDD scenarios (platform-e2e), then archive. Proto contract + vendoring (tasks 1-2) are done |

## Blocker legend & how to clear

- **A. Running-stack e2e + `make -C platform-e2e features-check`** — bring the stack up
  (`docker compose -f docker-compose.services.yaml up --build`), run the suite, flip the named feature flags
  to `automated`.
- **B. `go build`/`go test ./...` in Docker/CI** — Go isn't on this host; checked by the service Dockerfiles / CI.
- **C. Live-cluster verification** — kind cluster + ArgoCD sync.
- **D. `docker-compose.services.yaml`** — orchestrator-reserved (single-writer); batch edits in one integration pass.
- **E. Cross-change dependencies** — archive in dependency order (`plans/<id>/slices.yaml` `depends_on`).
- **F. Final step** — every change's last task is `openspec archive <id>`; run it only after its A-E are green.

## Verdict
**0/1 archivable today** (6 of 8 tasks unchecked). Build it through `feature-run` (see `docs/ONBOARDING.md`),
then archive after `features-check` and the Feature gate are green.
