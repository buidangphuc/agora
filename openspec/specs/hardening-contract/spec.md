# hardening-contract Specification

## Purpose
Contract for privacy-audit-hardening: the ReviewHidden engagement event and the voucher.admin_create audit action, kept identical across vendored proto copies.

## Requirements

### Requirement: ReviewHidden is an additive engagement event

`platform/engagement/v1/events.proto` SHALL define message `ReviewHidden` with exactly the fields `review_id` (string, 1), `listing_id` (string, 2) and `hidden_at` (google.protobuf.Timestamp, 3), and SHALL NOT change or renumber any existing message or field. The message SHALL carry no reviewer identifier and no review text.

#### Scenario: Hardening contract: ReviewHidden has the agreed fields
- **WHEN** the compiled descriptor of `platform.engagement.v1.ReviewHidden` is inspected
- **THEN** it has exactly 3 fields: `review_id` string number 1, `listing_id` string number 2, `hidden_at` `google.protobuf.Timestamp` number 3
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (descriptor check via `buf build` and `buf` JSON image)

#### Scenario: Hardening contract: ReviewHidden carries no user data
- **WHEN** the field names of `ReviewHidden` are listed
- **THEN** none of them is `user_id`, `reviewer_id`, `text`, `title` or `body` (0 matches)
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (forbidden-field assertion)

#### Scenario: Hardening contract: Change is backward compatible (guard)
- **WHEN** `buf breaking --against` the main branch runs on `platform-core/packages/proto`
- **THEN** it exits 0 with 0 findings
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (runs `buf breaking`); guard scenario, green at base by design

#### Scenario: Hardening contract: Removing a ReviewHidden field is rejected
- **WHEN** a synthetic copy of the proto drops `listing_id` or renumbers `hidden_at`
- **THEN** `buf breaking` against the real source exits non-zero with at least 1 finding
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (negative case on a temp tree)

#### Scenario: Hardening contract: Source protos lint clean
- **WHEN** `buf lint` runs in `platform-core/packages/proto`
- **THEN** it exits 0 with 0 findings, including for `ReviewHidden`
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (runs `buf lint`)

### Requirement: Audit catalogue lists voucher.admin_create

The action catalogue comment in `platform/audit/v1/audit.proto` SHALL state `Action catalogue (18)` and SHALL list `voucher.admin_create` together with the 17 existing actions. The messages, fields and service definitions of the file SHALL NOT change.

#### Scenario: Hardening contract: Catalogue has 18 actions
- **WHEN** the catalogue comment block of `audit.proto` is parsed
- **THEN** the header says `(18)`, the list contains 18 distinct actions, and `voucher.admin_create` is one of them
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (comment parse and count)

#### Scenario: Hardening contract: Audit wire format is unchanged (guard)
- **WHEN** `buf breaking --against` main runs and the descriptor of `audit.proto` is compared with main's
- **THEN** 0 findings, and the set of messages, fields and RPCs is identical
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (breaking plus descriptor diff); guard scenario, green at base by design

#### Scenario: Hardening contract: Catalogue count mismatch is rejected
- **WHEN** a synthetic `audit.proto` header says `(18)` but lists 17 actions
- **THEN** the catalogue check exits non-zero and names the mismatch
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (negative case on a temp file)

### Requirement: Consumer copies stay byte-identical

Every vendored copy of the changed files SHALL be byte-identical to the platform-core source: `events.proto` in team-engagement, team-analytics and team-order, `audit.proto` in all 11 repos that vendor it. Consumers SHALL pass `buf lint` on their proto module and SHALL compile with the new files.

#### Scenario: Hardening contract: Copies match the source
- **WHEN** every vendored `events.proto` (team-engagement, team-analytics, team-order) and `audit.proto` (11 repos) is compared with the source
- **THEN** all 14 comparisons report identical content
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (cmp over copies, same approach as `consumer_copies_test.sh`)

#### Scenario: Hardening contract: Drifted or missing copy is rejected
- **WHEN** one vendored copy differs by one byte or is missing in a synthetic tree
- **THEN** the copy check exits non-zero and prints the path of that copy
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (synthetic tree, as in `consumer_copies_test.sh`)

#### Scenario: Hardening contract: Consumer proto modules lint and build
- **WHEN** `buf lint` and `buf build` run in the proto module of team-engagement and of team-analytics
- **THEN** each exits 0 with 0 findings
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (loops over the two consumer modules)

#### Scenario: Hardening contract: Generated ReviewHidden type exists in consumers
- **WHEN** `buf generate` has run in team-engagement and team-analytics
- **THEN** each repo's generated Go package exposes type `ReviewHidden` with getters `GetReviewId`, `GetListingId`, `GetHiddenAt`, and `go build ./...` exits 0
- **VERIFIED BY**: platform-core `packages/proto/pah_contract_test.sh` (runs `buf generate` itself in both repos, greps the generated code, runs `go build`)
