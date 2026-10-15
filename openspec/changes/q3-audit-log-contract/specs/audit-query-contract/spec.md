## ADDED Requirements

### Requirement: QueryAuditLogRequest carries target, action and time filters

`QueryAuditLogRequest` in `platform/audit/v1/audit.proto` SHALL keep `actor_id = 1`, `target_type = 2` and `page = 3` unchanged and SHALL add `optional string target_id = 4` (exact match), `optional string action = 5` (exact match), `google.protobuf.Timestamp created_after = 6` (inclusive, unset = no lower bound) and `google.protobuf.Timestamp created_before = 7` (exclusive, unset = no upper bound). The RPC comment SHALL state that all filters AND together, that `created_before` earlier than `created_after` is `InvalidArgument`, that results are newest first with the existing paging, and that the RPC is admin-only.

#### Scenario: New filter fields exist with fixed numbers
- **WHEN** the compiled `QueryAuditLogRequest` descriptor is inspected
- **THEN** `target_id` is field 4 and `action` is field 5 (both string, proto3 optional), `created_after` is field 6 and `created_before` is field 7 (both `google.protobuf.Timestamp`), and `actor_id = 1`, `target_type = 2`, `page = 3` keep their names and types
- **VERIFIED BY**: Go descriptor test `platform-core/packages/proto/platform/audit/v1/auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Filters in every generated language
- **WHEN** `buf generate` runs on the proto
- **THEN** the Go, Python, JVM and TS outputs each expose `target_id`, `action`, `created_after` and `created_before` on `QueryAuditLogRequest`
- **VERIFIED BY**: `make -C platform-core proto` followed by a grep of each generated tree (tasks 2.2)

#### Scenario: Round-trip of equal time bounds
- **WHEN** a `QueryAuditLogRequest` with `created_after` equal to `created_before` is serialised and parsed
- **THEN** it parses back unchanged (the wire carries validation-free values; `InvalidArgument` for an inverted range is enforced by team-audit)
- **VERIFIED BY**: Go round-trip test `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Round-trip of empty action and long target id
- **WHEN** a `QueryAuditLogRequest` with an empty-string `action` and a 256-char `target_id` is serialised and parsed
- **THEN** it parses back unchanged and the empty `action` stays set (presence kept)
- **VERIFIED BY**: Go round-trip test `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Untouched request leaves new fields unset
- **WHEN** an empty `QueryAuditLogRequest` is serialised and parsed
- **THEN** `target_id`, `action`, `created_after` and `created_before` are all unset
- **VERIFIED BY**: Go round-trip test `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Old client request still parses
- **WHEN** bytes of a request that only sets `actor_id`, `target_type` and `page` (as produced before this change) are parsed with the new descriptor
- **THEN** parsing succeeds, those three fields are unchanged and the four new fields are unset
- **VERIFIED BY**: Go test `auditquery_roundtrip_test.go` (tasks 2.1) plus `make -C platform-core breaking`

#### Scenario: Filter semantics are documented
- **WHEN** a reader opens the `QueryAuditLog` RPC comment and the four new field comments
- **THEN** they state exact match for `target_id` and `action`, `created_after` inclusive, `created_before` exclusive, AND semantics, `InvalidArgument` when `created_before` is earlier than `created_after`, and admin-only access
- **VERIFIED BY**: comment-text assertion in `auditquery_roundtrip_test.go` (tasks 2.1) plus comment review (tasks 1.2)

#### Scenario: Change is backward compatible
- **WHEN** `buf breaking --against` the committed HEAD runs on the proto
- **THEN** it exits 0 with no findings
- **VERIFIED BY**: `make -C platform-core breaking`

#### Scenario: Wire-breaking edit is rejected
- **WHEN** a field number, name or type of any `QueryAuditLogRequest` or `WriteAuditEventRequest` field is changed or removed
- **THEN** `buf breaking` fails and the check does not pass
- **VERIFIED BY**: `make -C platform-core breaking` plus the descriptor assertions in `auditquery_roundtrip_test.go` (tasks 2.1, 1.4)

#### Scenario: Lint passes
- **WHEN** `buf lint` runs on the proto
- **THEN** it exits 0
- **VERIFIED BY**: `make -C platform-core lint-proto`

#### Scenario: Non-admin access is documented as denied
- **WHEN** a reader checks the `QueryAuditLog` RPC comment for a non-admin caller
- **THEN** it states `PermissionDenied` for authenticated non-admins and `Unauthenticated` for anonymous callers
- **VERIFIED BY**: comment-text assertion in `auditquery_roundtrip_test.go` (tasks 2.1); enforcement is verified in audit-query-filters

### Requirement: Audit action naming rule and catalogue are documented

The `WriteAuditEventRequest` comment SHALL state the action naming rule `<entity>.<verb>`, state that every call by a caller holding `admin` (team-order: `order.admin`) to a catalogue RPC is audited even when that caller owns the resource, and list the catalogue: `review.hide`, `kyc.review`, `dispute.resolve`, `listing.admin_update`, `listing.admin_delete`, `order.force_fail`, `order.admin_status_update`, `return.admin_status_update`, `shipment.admin_create`, `payment.admin_refund`, `voucher.platform_create`, `flash_sale.admin_create`, `ad_campaign.admin_create`. The wire of `WriteAuditEventRequest`, `WriteAuditEventResponse` and `AuditEvent` SHALL NOT change.

#### Scenario: Naming rule and catalogue are present
- **WHEN** a reader opens the `WriteAuditEventRequest` comment
- **THEN** it contains the text `<entity>.<verb>` and each of the 13 action strings above
- **VERIFIED BY**: 14-string comment assertion in `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Write wire is unchanged
- **WHEN** the descriptors of `WriteAuditEventRequest` (`actor_id = 1`, `action = 2`, `target_type = 3`, `target_id = 4`, `metadata = 5`), `WriteAuditEventResponse` (no fields) and `AuditEvent` (fields 1-7) are inspected
- **THEN** every field keeps its name, number and type and no field is added or removed
- **VERIFIED BY**: descriptor assertion in `auditquery_roundtrip_test.go` (tasks 2.1) plus `make -C platform-core breaking`

#### Scenario: Action missing from the catalogue is caught
- **WHEN** the comment on `WriteAuditEventRequest` lacks the naming rule or any one of the 13 action strings
- **THEN** the comment assertion fails and the contract slice is not done
- **VERIFIED BY**: 14-string comment assertion in `auditquery_roundtrip_test.go` (tasks 2.1)

#### Scenario: Uniform admin-path wording and non-admin exclusion
- **WHEN** a reader opens the `WriteAuditEventRequest` comment
- **THEN** it states that a caller holding the `admin` (team-order: `order.admin`) scope is audited even when they own the resource, and that a caller without that scope (permission denied for the admin path) is never audited
- **VERIFIED BY**: comment-text assertion in `auditquery_roundtrip_test.go` (tasks 2.1)

### Requirement: Proto copies are identical across repos

The updated `audit.proto` SHALL be copied byte-identical to `proto/platform/audit/v1/audit.proto` in team-audit, team-gateway, team-analytics, team-engagement, team-frontend, team-verification, team-domain, team-order, team-payment and team-promotion. Consumers SHALL NOT edit their copy.

#### Scenario: Copies match the source
- **WHEN** each of the ten copies is compared with `platform-core/packages/proto/platform/audit/v1/audit.proto`
- **THEN** all ten comparisons report identical content (exit 0)
- **VERIFIED BY**: `cmp` of the ten copies against the source (tasks 2.3)

#### Scenario: Drifted or missing copy is rejected
- **WHEN** any consumer copy differs from the source by one byte, or the file is missing
- **THEN** the `cmp` check exits non-zero and the contract slice is not done
- **VERIFIED BY**: `cmp` of the ten copies against the source (tasks 2.3)

#### Scenario: Consumer copies lint where a buf module exists
- **WHEN** `buf lint` runs in the `proto` directory of each copy owner that has a `buf.yaml` (team-payment has none: its copy is verified by `cmp` only)
- **THEN** each exits 0 with the new fields present
- **VERIFIED BY**: `buf lint` per consumer proto directory with a `buf.yaml` (tasks 2.3)

### Requirement: Producers receive the audit address in compose

The root `docker-compose.services.yaml` SHALL set `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` in the environment of team-verification, team-domain, team-order, team-payment and team-promotion, and SHALL keep the existing value on team-engagement. No other service SHALL gain the variable.

#### Scenario: Six producers have the audit address
- **WHEN** `docker compose -f docker-compose.services.yaml config --format json` is rendered and each service environment is read
- **THEN** `team-verification`, `team-domain`, `team-order`, `team-payment`, `team-promotion` and `team-engagement` each have `UPSTREAM_AUDIT_ADDR` equal to `team-audit-svc:50066`, and exactly those six services have it
- **VERIFIED BY**: rendered-config assertion script run in tasks 3.1 (reads only the root `docker-compose.services.yaml`)

#### Scenario: Compose file stays valid
- **WHEN** `docker compose -f docker-compose.services.yaml config -q` runs
- **THEN** it exits 0
- **VERIFIED BY**: `docker compose config -q` (tasks 3.1)

#### Scenario: Wrong or missing address is detected
- **WHEN** any of the six producers has `UPSTREAM_AUDIT_ADDR` missing or not equal to `team-audit-svc:50066`, or a service outside the six has it
- **THEN** the rendered-config assertion exits non-zero and the contract slice is not done
- **VERIFIED BY**: rendered-config assertion script run in tasks 3.1

### Requirement: Workspace rule that admin mutations audit first

Root `AGENTS.md` SHALL gain a rule in section 3: an RPC called by a caller holding the `admin` (team-order: `order.admin`) scope, including an admin who owns the resource, SHALL write a team-audit event (naming actor, `<entity>.<verb>` action and target) before applying the change, and SHALL return `Unavailable` with no change when the audit write cannot be made. `.claude/agents/auth-scope-reviewer.md` SHALL list the violation as a BLOCKING item and its CLEAN output SHALL name the admin-path RPCs it checked for an audit write.

#### Scenario: AGENTS.md carries the rule
- **WHEN** a reader opens section 3 of `AGENTS.md`
- **THEN** it holds the rule text naming `admin`/`order.admin`, "including an admin who owns the resource", "team-audit event first", `Unavailable` and "no change", and the existing rules 1-5 are unchanged
- **VERIFIED BY**: grep assertions on `AGENTS.md` (tasks 3.2)

#### Scenario: Reviewer flags an unaudited admin mutation
- **WHEN** the auth-scope-reviewer reads a diff in which an RPC mutates state only because the caller holds `admin` and no audit write precedes the mutation
- **THEN** its definition instructs it to report a BLOCKING finding, and to report a BLOCKING finding as well when the audit failure is swallowed so that the mutation still proceeds
- **VERIFIED BY**: grep assertions on `.claude/agents/auth-scope-reviewer.md` for the check text and the fail-closed wording (tasks 3.3)

#### Scenario: Owner path is not flagged
- **WHEN** the reviewer reads a diff where the same RPC is reachable by a resource owner who lacks the `admin`/`order.admin` scope
- **THEN** its definition states that path needs no audit write and is not a finding, while a caller holding the admin scope is audited even when they own the resource
- **VERIFIED BY**: grep assertion on `.claude/agents/auth-scope-reviewer.md` for the owner-path exemption (tasks 3.3)
