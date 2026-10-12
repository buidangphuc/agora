# Design

## Clarifications
### Round 1 — 2026-10-12
- Q1 [outcome] 18 code-track scenarios without e2e: how does spec_sync --strict count them? → **Each carries a `**VERIFIED BY**` line naming its unit test, with a `status: not-testable` FEATURES.yaml entry** (human)
- Q2 [outcome] "Denied buyer is served popular recommendations": the popular path (served by team-ai) already satisfies it before implementation → **Keep the e2e as a regression test; red-first exempt because the behaviour pre-exists outside this slice** (default accepted)
- Q3 [outcome] Nearline `consent_denied` / `decode_errors` counters: where observed? → **As `consent_denied N` / `decode_errors N` in the nearline consumer log** (default accepted)
- Q4 [data] Path of the v2 interaction snapshot → **`datasets/als_interactions/v2/`, same layout as v1** (default accepted)
