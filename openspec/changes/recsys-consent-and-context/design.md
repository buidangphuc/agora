# Design

## Clarifications
### Round 1 — 2026-10-12
- Q1 [outcome] 18 code-track scenarios without e2e: how does spec_sync --strict count them? → **Each carries a `**VERIFIED BY**` line naming its unit test, with a `status: not-testable` FEATURES.yaml entry** (human)
- Q2 [outcome] "Denied buyer is served popular recommendations": the popular path (served by team-ai) already satisfies it before implementation → **Keep the e2e as a regression test; red-first exempt because the behaviour pre-exists outside this slice** (default accepted)
- Q3 [outcome] Nearline `consent_denied` / `decode_errors` counters: where observed? → **As `consent_denied N` / `decode_errors N` in the nearline consumer log** (default accepted)
- Q4 [data] Path of the v2 interaction snapshot → **`datasets/als_interactions/v2/`, same layout as v1** (default accepted)

### Round 2 — 2026-10-12
- Q5 [outcome] Ranker debias: how is the (item_list_id, position) propensity estimated? (the existing position-only IPS already satisfied the three scenarios) → **Empirical per list: smoothed CTR (clicks+1)/(impressions+2) at the position divided by the same at position 1 in that list, ratio clamped to [0.05, 1], weight = 1/propensity; a cell or its position-1 cell under 100 impressions falls back to the position-only weight. Two scenarios added (VERIFIED BY)** (human chose the approach; thresholds are the default)
