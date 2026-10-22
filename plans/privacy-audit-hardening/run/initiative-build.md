# initiative-build privacy-audit-hardening
integration: .claude/worktrees/privacy-audit-hardening (asdlc/privacy-audit-hardening, base origin/main)
LOCK: approved 2026-10-18 (Bùi Đăng Phúc), digests verified; packets validated by scripts/validate_plan.py (12 packets, 4 waves, clean; warning: wave 1 has 7 parallel tasks).

| wave | slices | base | status |
|---|---|---|---|
| 0 | pah-contract | asdlc/privacy-audit-hardening | pending |
| 1 | analytics-review-hidden, domain-consumer-key-redaction, engagement-consumer-key-redaction, frontend-cookie-secure, identity-session-retention, payment-refund-audit-id, promotion-shop-voucher-audit | asdlc/privacy-audit-hardening | pending |
| 2 | engagement-review-hidden-event, featurestore-hidden-review-exclusion, frontend-session-client-context | asdlc/privacy-audit-hardening | pending |
| 3 | frontend-referrer-hygiene | asdlc/privacy-audit-hardening | pending |

## Held slices
(none)

## Wave moves (validate_plan same-wave overlap)
- frontend-referrer-hygiene: DAG level 2 -> wave 3 (overlaps frontend-session-client-context in wave 2: team-frontend/FEATURES.yaml; LOCK also names them a hot-file pair). Rule: the later slice in LOCK build order moves.

## Notes
- validate_plan.py cuts write-set entries at "(", so `team-frontend/src/app/(shop)/...` entries parse as the prefix `team-frontend/src/app/` (over-wide audit for frontend-referrer-hygiene; harmless while it is alone in wave 3).
