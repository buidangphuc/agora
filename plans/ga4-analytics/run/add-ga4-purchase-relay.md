# feature-run add-ga4-purchase-relay
initiative: ga4-analytics | base: 7c1ccd77 | worktree: .claude/worktrees/add-ga4-purchase-relay | branch: asdlc/add-ga4-purchase-relay | started: 2026-10-12 | packet: plans/ga4-analytics/wave3-task1-add-ga4-purchase-relay.md
## Preflight   validate=pass spec_lint=pass (1 PERM warn) spec_sync=35 uncovered | digest 5068777f68ed = LOCK
## E2E         author=done run=red as expected (relay 32 red: service missing; browser 1 red + 2 regression green) files=8 | commit c4812955; spec-clarify R1 → re-lock 41d8a0b4a2fe (4f72a307)
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
| cfg | Disabled without secret | approved | 2 | e5aeedfa | PASS | none | approve (R2) | - |
| mapping | 5 mapping scenarios | approved | 1 | 2c4aea9e | PASS | none | approve (3 NIT) | - |
| client | 7 MP client scenarios | approved | 1 | a0967e11 | PASS | none | approve (4 NIT) | - |
| relay | 15 relay scenarios | approved | 1 | b7bd1132 | PASS | none | approve (4 MINOR, 1 NIT: at-least-once window, DLQ holds buyer_id) | privacy at gate |
| purge | privacy follow-up: physical purge of expired relay rows + DLQ retention alter-config (user 2026-10-12) | approved | 1 | d8129de2 | PASS | none | approve (2 NIT: unchecked Purger assertion, DuckDB pool not capped) | - |
| ops | runAsNonRoot/read-only pod, CI runs gitops tests, local compose ga4-relay + DLQ topic, vault SERVICES (user 2026-10-12) | approved | 1 | 7c06afbf | PASS | none | approve (NIT: vault kv put overwrites hand-set GA4 keys on re-run; yq on runner unverified) | - |
| cmd | Secret unset; probes only | approved | 1 (+MINOR fixes) | 0b23f2ea | PASS | none | approve (2 MINOR fixed: fetch-error record loss, commit retry) | - |
| fe-map | 3 FE scenarios | approved | 1 | d58f8bb0 | PASS | none | approve (3 NIT) | - |
| gitops | 4 gitops scenarios | approved | 2 | f71370fd | PASS (REDFIRST_PYTHON) | none | approve R2 (Vault server/path hardcoded MINOR) | NEEDS_HUMAN: Vault role, CI wiring, runAsNonRoot |
## Gates       e2e=green (relay 30/30 after test fixes 46c3295f; browser 3/3; ui_checkout_order, buyer_full_funnel_journey, cart_checkout pass) features-check=pass spec_sync --strict=pass (32 e2e + 3 VERIFIED BY) audit=clean (43 files) privacy=pass-with-NEEDS_HUMAN (no physical purge of expired relay rows; DLQ retention only at create; DLQ copies not purged on denial; revocation lag until next event; cross-border + order_id basis need DPO)
## Escalations
