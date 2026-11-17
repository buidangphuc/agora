# Design

## Clarifications
### Round 1 - 2026-11-13
- Canonical source is `platform-core/packages/proto/platform`; every other copy is replaced byte-identical, never edited.
- `PrivacyService` lives in `identity.proto` (spec: identity owns the consent ledger and the deletion lifecycle). `order.proto` already imports `identity.proto` and `payment.proto`, so adding messages to identity does not create an import cycle.
- buf lint requires enum values prefixed with the enum name: `CONSENT_PURPOSE_*` and `USER_STATUS_DELETED` are already prefixed; `VERIFICATION_STATUS_NOT_SUBMITTED` follows the same rule. Numbers are exactly the spec's.
- `ReviewKycRequest` has `id = 1` and `decision = 2`, so the spec's `reason_code = 3` is the next free number and is kept. The same holds for `SubmitKycRequest.additional_doc_refs = 3` (fields 1-2 used) and `GetVerificationStatusResponse.reject_reason_code = 3` (fields 1-2 used). 
- Principals, scopes and the 501 rule cannot be enforced by a proto file; each is stated in an RPC comment and tested by the owning behaviour slice. The Go test asserts the comment phrases, like the q7 contract test.
- The audit catalogue comment is a pinned string in `gaps_contract_docs_test.go` ("Action catalogue (25)"); this slice updates that pin to 26 and appends `kyc.document_view` to the list, otherwise the existing test goes red.
- `UserErased` goes next to `UserStatusChanged` in `identity.proto`, same envelope convention (`platform.events.v1.EventEnvelope`).
- Test location: `platform-core/packages/proto/platform/identity/v1/trust_legal_compat_test.go` (reads identity, order, payment, verification, audit protos through one `buf build`); copy check: `platform-core/packages/proto/trust_legal_consumer_copies_test.sh`, mirroring `q7_consumer_copies_test.sh`.
- Copy lists are taken from the repos that carry each proto today (checked with `ls */proto/platform/<name>/v1`). team-verification has neither `identity.proto` nor `events.proto` today and gains both (for `UserErased`); the erasure consumers in team-engagement, team-chat, team-notification, team-domain and team-analytics already carry both. Any further copy a behaviour slice needs is added by that slice and re-checked with the same `cmp` script.
