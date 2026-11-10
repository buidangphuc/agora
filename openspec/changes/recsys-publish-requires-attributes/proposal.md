# Proposal: recsys-publish-requires-attributes

## Why

After discovery-seo was promoted to the local main env (3329105d, 2026-11-09), team-ai drops every candidate whose
payload has no `visible` key (`team-ai/app/modules/business/recommendation/ranking/filter.py` `eligible`, spec
`recsys-candidate-attributes` "Consumers treat absent visibility as not eligible"). platform-recsys writes `visible`
only when an `item_attributes@v2` snapshot exists; without one it publishes the old payload shape
(`platform-recsys/recsys/load/attributes.py:8`, scenarios "Missing snapshot does not abort the publish" and "Corrupt
snapshot does not abort the publish"). A generation trained without the snapshot therefore serves nothing, the popular
fallback included. On the main env this turned these e2e files red: `test_ple_placement_engine`, `test_ple_home_feed`,
`test_gbl_ranker_serving`, `test_mla_nearline_ctr`, `test_ssa_serving_switch_atomicity`, `test_rss_serving_safeguards`,
`test_rss_recommendation_attribution`, `test_recsys_consent_context`, `test_recommendations`,
`test_tpr_trained_recs_local`, `test_ai_auth`, the recommendation rows of `test_d1_discovery`, `test_d1_pdp` and
`test_buyer_full_funnel_journey`.

Decided by the human (2026-11-10): keep the consumer fail-closed; the producer refuses a training run that has no
readable snapshot, before training, with exit 2 (the config-error code of the job), so an empty generation can never
reach serving.

## What changes

- platform-recsys: a training run resolves the `item_attributes@v2` snapshot first; missing or unreadable → exit 2,
  `attributes=missing`, no MLflow run, no store write, serving pointer untouched. Stale snapshots still train (warning
  unchanged). `rollback` and `backfill-attributes` are unchanged.
- platform-e2e: the recsys job driver (`tests/e2e/flows/recsys_job_driver.py`) writes an `item_attributes@v2`
  snapshot marking every fixture listing `LISTING_STATUS_PUBLISHED` for a run that does not set `@item_attributes`, so
  the older ML scenarios train as before; a run that sets `"@item_attributes": null` gets no snapshot. One new
  scenario proves a refused run keeps the serving generation.

## NFR

- The snapshot check reads one Parquet footer before the dataset read; a refused run ends within seconds and holds no
  Spark session.
- Availability: a refused run leaves the serving generation as it was; serving never sees a generation without
  `visible`.

## Data classification

No field is added. The check reads the existing `item_attributes@v2` snapshot (ids, `seller_id`, `category_id`,
`status`, classified in the archived change `recsys-candidate-attributes`). Logs carry `attributes=missing` and counts,
never an id. The e2e driver writes only synthetic fixture ids.

## Threat model

- Asset: the guarantee that recommendations never come back empty because a producer skipped the attributes, and that
  an unpublished listing is never recommended.
- Abuse or failure cases: (1) the featurestore job fails and the nightly run trains anyway, publishing an empty
  generation: mitigated, the run exits 2 before training; (2) a corrupt or truncated snapshot is treated as valid:
  mitigated, unreadable Parquet or a missing `listing_id` column refuses; (3) the refusal is mistaken for success by
  the chain: exit 2 is non-zero and logged.
- Authz: no request path is added; the job keeps its existing store credentials.

## Impact

- Capability `recsys-candidate-attributes`: requirement "Item payloads carry seller, category and visibility" loses the
  two "does not abort the publish" scenarios; new requirement "A run without a readable attribute snapshot refuses
  before training".
- Deploy order: the featurestore job that writes `item_attributes@v2` must run before the nightly recsys run (already
  the chain order of `recsys-auto-train`). A generation already serving in the old shape stays empty until the next
  run or `backfill-attributes`.
- Data classification: unchanged (ids and one boolean; logs carry counts, no ids).
