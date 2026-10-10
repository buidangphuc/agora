# Decisions — AI-first program (C1 / C2 / C3)

Status: open, 2026-10-01. One merged list of the user decisions in the three OpenSpec changes
(`tracking-event-platform` = C1, `analytics-feature-store` = C2, `recommendations-end-to-end` = C3).
Every change's specs and tasks are written against the **recommended default**; changing a decision
edits only the tasks listed under "Affects". Ids keep the change's own number (`C1-U2` = C1 design U2);
merged rows list every source id.

Already settled by the program planner (not open): serving proto fields owned by C1 (was C1-U9);
ADR numbers C1 = 0014, C2 = 0015, C3 = 0016 (was C3-U6); `feature_failure` and
`<entity>.<name>@v<N>` everywhere; raw-event retention owned by C1.

## Privacy and data collection

| Id | Decision | Recommended default | Alternative | Affects |
|---|---|---|---|---|
| C1-U1 | Consent model **(legal)** | Opt-out: tracking on by default, account toggle + footer link, `Sec-GPC` honoured; both modes built (`TRACK_CONSENT_MODE`), switch is config | Opt-in banner (no behavioural events until consent) | C1 3.6, 3.11, 4.7; C2 8.8; C3 5.11 (behaviour unchanged, only the share of opted-out traffic) |
| C1-U2 + C2-U6 | Retention **(legal)** | Raw (C1): behavioural 13 mo, free text nulled 90 d, identity links 13 mo after last seen, facts 25 mo, quarantine 14 d, Kafka 7/14 d. Feature store (C2): offline 180 d, datasets 30 d (180 d pinned), online by TTL | Behavioural 6 mo (loses year-over-year features); datasets kept forever when pinned | C1 6.2; C2 8.5 |
| C1-U3 | Store search queries | Scrubbed queries (email/phone/CMND-CCCD masked at the edge), nulled after 90 d | Hash + token count only (kills query features, LTR) | C1 3.5, 6.2 |
| C1-U4 | Track anonymous visitors | Yes: random first-party `bds_aid` cookie (13 mo) + stitching at login | Logged-in only (anonymous recs stay popularity-only) | C1 4.1, 5.4 |
| C1-U8 | Who triggers erasure | Admin RPC `EraseSubject` + runbook (calls C2's hook) | `UserDeleted` event from team-identity now (new outbox) | C1 6.3; C2 8.4 |
| C3-U7 | Personal lists for anonymous visitors | Not written (anon interactions still train item vectors; anon gets similar-to-recent) | Write `anon:` lists | C3 4.5 |

## Tracking scope

| Id | Decision | Recommended default | Alternative | Affects |
|---|---|---|---|---|
| C1-U5 | Warehouse layout | Hybrid: one wide `tracking_events` + typed fact tables | All per-type tables, or all wide | C1 5.x, 9.1 |
| C1-U6 | Sampling | None (all 1.0); deterministic per-session knobs; never sample IDENTIFY / CONSENT_UPDATE / CHECKOUT_START / ADD_TO_CART | Impressions at 0.25 (CTR features need reweighting) | C1 3.7 |
| C1-U7 | Server-side fact topics | `order.events` + `engagement.events` + consume `payment.events` | Also returns, Q&A, collections, cart; or orders only | C1 7.x, 8.x, 9.1 |
| C3-U10 | Exposure logging | Client-side impressions/clicks via the C1 SDK; team-ai emits no served event | Server-side "served" event (second producer on `analytics.events`) | C1 non-goal; C3 9.2, 10.9 |

## Feature store

| Id | Decision | Recommended default | Alternative | Affects |
|---|---|---|---|---|
| C2-U1 | Online read path | gRPC `GetOnlineFeatures` (staleness, versions, authz in one place) | Consumers `HGET fs:v1:*` directly (valkey becomes a shared DB, Rule 3) | C2 6.x, 11.x; C3 5.9 |
| C2-U2 | Online freshness | 60 s micro-batch of the same SQL ⇒ ~2 min SLO (replaces P2-T4's < 10 s) | 10 s for `trending_1h` / `recent_items_24h` (~6x query load) | C2 3.6, 3.7 |
| C2-U3 | Offline store | One `features` bucket on MinIO (S3 when deployed) | GCS via BigQuery external tables | C2 3.4, 9.x, 10.x |
| C2-U4 | Registry format | One `features/registry.yaml` + JSON schema | One file per feature | C2 2.x |
| C2-U5 | Versioning | Immutable `@v<N>`; deprecation sunset ≥ 30 d | Semver `@v1.2` | C2 2.3, 2.5 |
| C2-U7 | Who adds features | Any team via PR, `team-analytics` CODEOWNERS approve | Analytics-only, or self-approved `experimental` | C2 2.6 |
| C2-U8 + C2-U11 | Dataset requests and scopes | `BuildDataset` / `GetDatasetBuild` RPCs; two service-only scopes `features.read` (team-ai), `features.dataset` (recsys trainer) | Drop view files in the bucket; one scope for everything | C2 1.1, 4.1, 5.5, 6.3; C3 4.1, 4.10 |
| C2-U9 | Where materializers run | Inside the team-analytics process, one replica, PVC | Separate Job on the Parquet mirror (+ up to 1 h delay) | C2 3.x, 9.x |
| C2-U10 | BigQuery | BI-only optional sink; DuckDB primary everywhere | BigQuery as alternative primary (two SQL dialects) | C2 3.1, 3.2 |
| C2-U12 + C3-U12 | Trending under a key-based read | C3 re-scores a candidate pool by `item.trending_1h`, cached 30 s | C2 adds `ListTopEntities(feature, k)` (contract change) | C3 5.7 |

## Recommendations

| Id | Decision | Recommended default | Alternative | Affects |
|---|---|---|---|---|
| C3-U1 + C3-U2 | Eligibility | team-ai asks team-search (`listing_ids = 10`), fail closed | New search RPC / own listing set; fail open | C3 1.2, 2.x, 5.5, 5.6 |
| C3-U3 | Gate thresholds | Recall@10 ≥ 0.01 deployed / 0.05 local; lift ≥ 1.0; coverage, overlap, min users/items as in C3 design | Stricter once real traffic exists | C3 4.4, 7.2, 8.3 |
| C3-U4 | Evaluation cost | Two fits per run (holdout fit for metrics, full fit for publish) | One fit (loses newest interactions) | C3 4.6 |
| C3-U5 | Cadence | Nightly 03:00 UTC; triggered build, 30 min wait, refuse watermark > 3 h; stale alert 36 h | Every 6 h | C3 4.1, 4.10, 8.3, 8.4 |
| C3-U8 | Online comparison | A/B by user hash with a challenger generation, 10 %, manual promote | Shadow scoring; interleaving | C3 4.8, 5.2, 10.6, 10.7 |
| C3-U9 | Placements served | `home.for_you`, `pdp.similar`, `home.trending` (rendered as a 2nd home row), minimal config | `home.trending` API-only; add `cart.cross_sell` | C3 5.4, 9.3, 10.6 |
| C3-U11 | Online-metrics surface | `GetPlacementMetrics`, admin-only (admin set 6 → 7) | Prometheus gauges only (no `request_id` drill-down) | C1 3.10; C3 1.3, 3.2, 6.2 |

## Needs legal review (before production rollout, not before building)

- **Consent mode (C1-U1).** Vietnam's Decree 13/2023/ND-CP on personal data protection and the Personal
  Data Protection Law may require explicit, opt-in consent for behavioural/profiling data. The default
  (opt-out + GPC) is an engineering default only; `TRACK_CONSENT_MODE=opt_in` is built and is a config
  switch. Also confirm: is anonymous-cookie tracking personal data processing requiring consent; is the
  account toggle + footer link sufficient notice.
- **Retention (C1-U2 + C2-U6).** 13 months behavioural, 25 months facts, 90 days free text, 180 days
  offline features / pinned datasets: confirm against the purpose-limitation and storage-limitation
  rules, and the seller-reporting / tax needs that motivate 25 months.
- **Opt-out vs history.** Default: opt-out stops collection and excludes the subject from new features,
  datasets and personalized serving, but does not purge history (erasure does). Confirm this is enough,
  or whether opt-out must trigger erasure of behavioural history.
- **Erasure SLA (C1-U8).** Raw rows and online features within 24 h, offline partitions and datasets
  within 30 days, Kafka residue 7–14 days: confirm against the statutory response deadline.
