# Design

## Clarifications

### Round 1 — 2026-10-15

- Dates: `from` and `to` are calendar days in UTC+07:00 and `to` is inclusive (`created_before` = `to` + 1 day at 00:00 +07:00). An unparseable date is refused locally without a gateway call. Event time is displayed as `YYYY-MM-DD HH:mm:ss` in UTC+07:00.
- Copy: the Vietnamese copy table in proposal.md is accepted as proposed.
- Proto: the contract slice (q3-audit-log-contract) adds `team-frontend/proto/platform/audit`; this slice only regenerates `src/generated`.
