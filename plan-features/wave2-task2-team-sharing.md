# [W2] F10 team-sharing (new service)

## Role
SE

## Objective
Create the NEW Go service repo **team-sharing/** end-to-end: scaffold by mirroring team-notification, implement the RPC(s) below with real DB logic + migration, and unit tests. Verify green on host.

## Write-set (EXCLUSIVE — the new dir only)
- team-sharing/** (new: go.mod, cmd/server/main.go, internal/{handler,service,repository,config}, Dockerfile, Makefile, buf.gen.yaml, proto/ (vendored), migrations/, generated/, *_test.go)

## Read-only dependencies
- platform-core proto package for this service (authored in Wave 0): platform/sharing/v1
- Reference service to COPY structure from: **team-notification/** (simplest committed-stub Go service). Same module layout, Dockerfile, Makefile, buf.gen.yaml, main wiring, interceptor/auth, config.

## Contracts you implement
`CreateShareLink(target_type, target_id, utm)` -> short_code; `ResolveShareLink(short_code)` -> {target, utm, og_meta}. OG meta = title/description/image for the target.

## Reference implementation
Stamp the team-notification structure into team-sharing/ (same file names + layering), then fill the slots for this domain. Data model: share_links(short_code pk, target_type, target_id, utm jsonb, created_at, click_count).. Vendor the Wave-0 proto into team-sharing/proto/, `buf generate`, implement handler→service→repository, add migration + tests.

## Acceptance criteria
- [ ] Repo compiles + `go test ./...` green (mirror team-notification's InMemory + Postgres repo split so tests run without a live DB)
- [ ] RPCs implement real logic; auth-scoped where user-owned; anonymous/empty handled
- [ ] >=3 unit tests
- [ ] go.mod module path + Dockerfile + Makefile present (mirrors team-notification)

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-sharing && (buf generate 2>/dev/null||true) && go test ./...

## Out of scope
- Do NOT edit team-gateway, team-frontend, platform-gitops, docker-compose (gitops chart + compose entry + gateway forwarder = integration wave).
- Do NOT edit platform-core proto (Wave 0 authored your package) or other service repos.
