# [W2] F11 team-audit (new service)

## Role
SE

## Objective
Create the NEW Go service repo **team-audit/** end-to-end: scaffold by mirroring team-notification, implement the RPC(s) below with real DB logic + migration, and unit tests. Verify green on host.

## Write-set (EXCLUSIVE — the new dir only)
- team-audit/** (new: go.mod, cmd/server/main.go, internal/{handler,service,repository,config}, Dockerfile, Makefile, buf.gen.yaml, proto/ (vendored), migrations/, generated/, *_test.go)

## Read-only dependencies
- platform-core proto package for this service (authored in Wave 0): platform/audit/v1
- Reference service to COPY structure from: **team-notification/** (simplest committed-stub Go service). Same module layout, Dockerfile, Makefile, buf.gen.yaml, main wiring, interceptor/auth, config.

## Contracts you implement
`WriteAuditEvent(actor, action, target, metadata)` (append-only, fire-and-forget), `QueryAuditLog(filters, page)` -> events. Immutable.

## Reference implementation
Stamp the team-notification structure into team-audit/ (same file names + layering), then fill the slots for this domain. Data model: audit_events(id, actor_id, action, target_type, target_id, metadata jsonb, created_at) — append-only, indexed by (actor_id, created_at) and (target, created_at).. Vendor the Wave-0 proto into team-audit/proto/, `buf generate`, implement handler→service→repository, add migration + tests.

## Acceptance criteria
- [ ] Repo compiles + `go test ./...` green (mirror team-notification's InMemory + Postgres repo split so tests run without a live DB)
- [ ] RPCs implement real logic; auth-scoped where user-owned; anonymous/empty handled
- [ ] >=3 unit tests
- [ ] go.mod module path + Dockerfile + Makefile present (mirrors team-notification)

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-audit && (buf generate 2>/dev/null||true) && go test ./...

## Out of scope
- Do NOT edit team-gateway, team-frontend, platform-gitops, docker-compose (gitops chart + compose entry + gateway forwarder = integration wave).
- Do NOT edit platform-core proto (Wave 0 authored your package) or other service repos.
