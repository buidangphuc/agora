## Why

Initiative `privacy-audit-hardening` (locked 2026-10-18, clarification Q8) makes a hidden review stop counting in analytics and recsys features. Admin moderation (`engagement-review-integrity`) hides a review, and `analytics-review-hidden` records a `review_hidden` fact, but the featurestore SQL still counts every `fact = 'review_created'`: `registry/sql/item_popularity.sql:27` (`review_count`, `avg_rating`), `als_interactions_v1.sql:35` and `als_interactions_v2.sql:51` (review weight +2.0/-2.0). A hidden abusive or fake review therefore keeps boosting or penalising a listing in popularity features and in ALS training data.

initiative: privacy-audit-hardening

## What Changes

Registry rules (`registry/features.yaml` header, `featurestore/registry.py`): each `(name, version)` is pinned by a SHA-256 in `registry/features.lock`; changing a locked SQL file fails the job with exit 4 ("changed without a version bump"). So no locked file is edited. New versions are added and the lock is regenerated with `python -m featurestore lock`:

- **`item_popularity@v2`** (new `registry/sql/item_popularity_v2.sql`): same output columns and semantics as v1, except the `rev` CTE ignores a `review_created` fact that has a `review_hidden` fact (same `review_id`, `occurred_at <= $as_of`). `item_popularity@v1` stays registered and unchanged (still counts hidden reviews) so existing consumers keep working until they move.
- **`als_interactions@v3`** (new `registry/sql/als_interactions_v3.sql`): `als_interactions@v2` (consent filter) plus the same hidden-review exclusion in `rev_latest`, applied before `row_number()` so a user's other, visible review of the same listing still counts. `als_interactions@v1` and `@v2` stay unchanged. Why v3 and not a hidden-only variant of v1: v2 is the consent-correct dataset; a new unconsented variant would re-create a privacy gap.
- **`featurestore/inputs.py`**: the `facts` view gets a nullable `review_id` column when the export lacks it (like `_add_missing_event_columns` does for events), so old exports still load. Rows with a NULL `review_id` cannot be matched to a hide and are counted as today.
- **Match rule**: a hide applies to a review when `review_hidden.review_id = review_created.review_id`. There is no unhide event in the contract, so a hidden review stays excluded.
- **README** (`platform-featurestore/README.md`): view/dataset tables list `item_popularity@v2` and `als_interactions@v3`; the facts input description lists `review_id`.
- **Not changed**: v1/v2 SQL and their lock hashes, `user_activity@v2`, `rank_training@v1/v2` (no review term), `item_attributes`, `user_preferences`, the proto, any RPC, the Redis key layout (keys are per version, so `item_popularity@v2` writes its own keys).
- **Consumers are not re-pinned here.** `platform-recsys` reads `als_interactions` v1 by default (`DATASET_DIR=/features/datasets/als_interactions/v1`); moving it to v3 is a platform-recsys/gitops change outside this slice (see Open items).

## Impact

- Depends on `analytics-review-hidden`: the exported `engagement_facts.parquet` must carry a `review_hidden` row (`fact='review_hidden'`, `listing_id`, `occurred_at`, `ingested_at`) and a `review_id` column populated for `review_created` and `review_hidden` rows. Confirmed by the orchestrator: `analytics-review-hidden` stores a nullable `review_id` on `engagement_facts` for both `review_created` and `review_hidden`. It is not in the fixtures' current schema, so the test fixtures gain it.
- Affected specs: new capability `hidden-review-exclusion` (additive).
- Cost: v3 and v2 views add one anti-join on `facts` per run; the nightly materialization (02:15) and dataset build stay within existing time budgets.

## NFR

- Latency/throughput: the added anti-join is over the `review_hidden` subset only (a few rows relative to `facts`); materialization and dataset build time grow by less than 5% on the fixture-scale and nightly inputs. Online read latency is unchanged (Redis lookup, under 10 ms).
- Availability: no new dependency. If the export has no `review_hidden` rows or no `review_id` column the job still succeeds and output equals the v1/v2 logic.
- Limits: point-in-time rule is preserved: only facts with `ingested_at <= AS_OF` (and `occurred_at <= $as_of`) are visible, so a hide recorded after AS_OF does not change an earlier as-of build (reproducibility of a stored dataset and its manifest hash).
- Failure behaviour: registry drift still exits 4; a changed locked file is never silently accepted.

## Data classification

- No new personal data. The SQL reads `review_id` (pseudonymous aggregate id, no user id, no review text) and `listing_id` from `review_hidden` facts; `review_created` rows already carry `user_id` (purpose: recsys training and item features; basis and retention unchanged from `governed-datasets`, owned by the DPO).
- Effect on personal data: excluding hidden reviews removes a user's review-derived interaction from newly built datasets and features (data minimisation for moderated content). Datasets already built keep their rows until the next build; stored manifests are not rewritten.
- Test data: synthetic ids (`u1`, `L1`, `R1`), no real user data.

## Threat model

- Assets: integrity of popularity features and of ALS training data; consent-correctness of `als_interactions`.
- Abuse cases:
  - Fake or abusive review inflates a listing's `review_count`/`avg_rating` or its ALS weight and is hidden by an admin, but keeps counting. Mitigation: exclusion by `review_id`; scenarios assert both outputs.
  - Hide of one review wrongly drops another review by the same user or on the same listing. Mitigation: match on `review_id`, not on `(user, listing)`; scenario asserts the sibling still counts.
  - Back-dated or later hide changes a historical dataset. Mitigation: point-in-time filter on `ingested_at`/`occurred_at <= $as_of`; scenario asserts an as-of before the hide is unchanged.
  - Editing a locked SQL file to slip in the change. Mitigation: lock hash check (exit 4); scenario asserts v1/v2 hashes unchanged.
  - New dataset version bypasses consent. Mitigation: v3 is built from v2; scenario asserts non-consented users are still absent.
- Authz rule: none at this layer (batch job, no RPC). The trust boundary is the export directory written by team-analytics; only the job's service account reads it.

## Open items

- Accepted limitation (orchestrator decision): legacy `review_created` rows with NULL `review_id` (stored before `analytics-review-hidden`) are never excluded, even if their listing has hides; there is no backfill.
- Which consumers move to the new versions: `platform-recsys` default `DATASET_DIR` (v1), and whoever reads `item_popularity@v1` Redis keys. Until they are re-pinned, v1 behaviour (hidden reviews still counted) remains live. Decision needed: retire `item_popularity@v1` and `als_interactions@v1`/`v2` after the move (precedent: `user_activity@v1` retired by `order-facts-buyer`) or keep them.
- No unhide event exists; if one is added later, the exclusion must be revisited.
- DPO sign-off on retained history in already-built datasets (shared with the initiative's pii routing).
