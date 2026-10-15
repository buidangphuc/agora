# Design

## Clarifications
### Round 1 — 2026-10-15
- Values, numbers and comments follow the Contracts section of `plans/q4-loop-instrumentation/00-spec.md`, verbatim.
- Copies: only team-gateway, team-analytics and team-frontend carry `analytics.proto`; all three are identical to the source today. No new copy locations.
- Test location: `platform-core/packages/proto/platform/analytics/v1/` beside `consent_compat_test.go`, same dynamic-descriptor approach.
- The `alert_type` and AI allowlist values are not restated in the proto comments beyond the key names; allowlist enforcement is a gateway concern.
- Human answer: the gateway rejects `ai_interaction` property keys other than `surface`/`action`; the value-21 proto comment may say "properties: exactly surface, action".
