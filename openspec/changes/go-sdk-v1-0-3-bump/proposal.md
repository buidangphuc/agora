## Why

go-sdk `v1.0.3` logs a keyed fingerprint `principal_fp` instead of the raw `principal_id`, and its shared sloglint config forbids `principal_id` (and the other raw id and contact keys) as a log attribute. Thirteen services (analytics, audit, chat, domain, engagement, notification, order, payment, promotion, referral, search, sharing, verification) still required `v1.0.2`, so every request log line they write carries the raw principal id. team-identity and team-gateway moved to `v1.0.3` in PR #93, but their specs still pin `v1.0.2`, so the specs drifted from the code. Fifteen requirements and twelve layout tests pinned the exact version, so every SDK bump has to rewrite specs and tests.

Decision by the user (2026-11-05): specs and tests stop pinning an exact version. The rule for every Go service is: `go.mod` requires go-sdk exactly once, at a tagged release version matching `^v\d+\.\d+\.\d+$` (no pseudo-version, no `-` suffix), has no `replace` directive and pins golangci-lint as a `tool`.

## What Changes

- All 15 Go services require `github.com/buidangphuc/agora/platform-core/packages/go-sdk v1.0.3` (`go mod tidy` applied; identity and gateway were already on it).
- The go.mod layout test of each service checks the tag rule instead of a literal version: exactly one require of the SDK module, a version matching `^v\d+\.\d+\.\d+$`, no `replace` directive and the golangci-lint `tool` line. team-notification and team-payment keep their negative fixture test, now with pseudo-version, pre-release, duplicate-require and missing-tool fixtures in place of the old "wrong version" fixtures.
- Each capability that pinned `v1.0.2` restates its requirement with the tag rule in place of the version clause. The audit and identity requirements had a scenario titled "The module requires the v1.0.2 tag"; a MODIFIED block cannot rename a scenario, so each is REMOVED and ADDED again under a new name with the scenario "The module requires a tagged SDK release" and every other clause and scenario unchanged. The gateway requirement is RENAMED because its title named the version. The `config.Load` clauses of engagement and promotion drop the version.
- The call-log requirements of identity, search and sharing said call logs carry "the principal id"; they now say `principal_fp`, never the raw id. team-sharing `TestCallLogsCarryNoLinkData` asserted the raw `principal_id` and failed on `v1.0.3`; it now asserts `principal_fp` equals `logx.PrincipalFP("user-1")` with no `principal_id` key and no raw id in the output, and `TestAnonymousCallIsLoggedAndServed` also refuses `principal_fp` on an anonymous call.
- READMEs of referral, sharing and verification describe the tag rule; the stale Known gaps bullets of identity and gateway ("v1.0.3 has no tag, `replace` to the local path") are deleted. FEATURES.yaml of analytics and audit follow the new scenario text and title.

No proto, RPC, schema, event, config key or runtime behaviour change other than the SDK log attribute (`principal_fp` replaces `principal_id`).

## Non-goals

- A go.mod layout test for team-promotion (it has no such requirement or test today).
- Rewording the team-payment ledger rationale that cites the `v1.0.2` `pagination.EncodeOffset` format: it states why the service keeps its own cursor codec, not a version pin.
- Archived changes and `plans/`.

## Impact

- Repos: all 15 Go `team-*` services (`go.mod`, `go.sum`), the go.mod layout tests of 14 services, the team-sharing call-log tests, READMEs of gateway, identity, referral, sharing and verification, FEATURES.yaml of analytics and audit.
- Capabilities: analytics-service-runtime, audit-service-runtime, chat-service, domain-go-sdk, engagement-sdk-runtime, gateway-service-runtime, identity-service-runtime, notification-service-runtime, order-service-runtime, payment-service-runtime, promotion-service-runtime, referral-service, search-service-runtime, sharing-service, verification-runtime.
- Log consumers: the request log attribute `principal_id` disappears from the 13 bumped services and `principal_fp` appears; a dashboard or query that filtered by `principal_id` must use `principal_fp` (`logx.PrincipalFP`) instead.
- Deploy: `LOG_KEY_FP_SECRET` keys the fingerprint; without it the SDK uses a random per-process key, so fingerprints are stable only within one process.

## Non-functional requirements

- Build: every service builds from the module cache with the tag (no `replace`, no `third_party`), so `docker build` from the service directory resolves the SDK.
- Lint: `make -C team-<svc> lint` runs the shared `../platform-core/packages/go-sdk/.golangci.yml`, whose sloglint `forbidden-keys` fail the build on a raw id or contact key.
- Tests: the go.mod layout tests fail on a pseudo-version, a `-` suffixed version, a second require, a `replace` directive or a missing golangci-lint `tool` line, and pass on any `vX.Y.Z` tag.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `principal_fp` (log attribute) | personal (keyed pseudonym of the principal id) | correlate one caller's log lines without the raw id | legitimate interest: operations and abuse investigation | platform log retention |
| `principal_id` (log attribute) | personal (pseudonymous id) | removed from logs | none | no longer written |

No new data is collected; the change reduces personal data in logs. No DPO sign-off is required (user rule: legitimate interest, platform log retention).

## Threat model

- Assets: request logs and the identity of callers in them.
- Abuse cases:
  - an operator or a log pipeline reader links log lines to a user through the raw principal id (removed: logs carry only the keyed fingerprint);
  - a service pins an untagged pseudo-version or a local `replace`, so the image builds from code no tag identifies (refused by the go.mod layout tests);
  - a new log call reintroduces `principal_id`, `user_id` or another raw id key (refused by the shared sloglint `forbidden-keys`).
- Authz rule: unchanged; the SDK bump changes no scope or principal check.
