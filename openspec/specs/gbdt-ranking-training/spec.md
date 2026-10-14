# gbdt-ranking-training Specification

## Purpose
Defines how the recsys batch run trains, gates and publishes a LambdaRank GBDT ranker from governed impression data and featurestore snapshots.

## Requirements

### Requirement: The batch run trains a GBDT ranker on governed impressions

When `ENABLE_GBDT` is true the recsys run SHALL train a gradient-boosted decision tree ranker with a LambdaRank objective on
the `rank_training@v1` dataset (`RANK_DATASET_PATH`, or the latest snapshot under `RANK_DATASET_DIR`), grouped by impression.
Each row's features SHALL come from the featurestore offline snapshots (`item_popularity@v1`, `item_attributes@v1`) with the
latest `as_of` not after the row's impression time, in the single order of `RANKING_FEATURES`; a missing or null value SHALL
be 0. A row with no such `item_popularity` snapshot SHALL be dropped and counted. With no dataset or no `item_popularity`
snapshot the run SHALL exit 2 before Spark starts, naming `RANK_DATASET_DIR` or `ITEM_FEATURES_DIR`, and register nothing.
With `ENABLE_GBDT` false the run SHALL be exactly as before.

#### Scenario: The trainer learns from clicks and beats the fixed-weight baseline

- **WHEN** the recsys job runs with the GBDT stage enabled over a ranking dataset whose clicks depend on item price
- **THEN** the run summary reports the GBDT candidate as promoted with an `ndcg@10` greater than the `baseline_ndcg@10` of the fixed-weight ranker

#### Scenario: The model's feature list is the serving contract

- **WHEN** the GBDT stage publishes its artifact
- **THEN** the artifact lists the features `item_popularity.views_7d`, `item_popularity.clicks_7d`, `item_popularity.add_to_cart_7d`, `item_popularity.favorites_current`, `item_popularity.review_count`, `item_popularity.avg_rating`, `item_popularity.ctr_7d` and `item_attributes.price`, in that order, with the view versions

#### Scenario: Each training row records where its CTR came from

- **WHEN** the GBDT stage runs with a path set for the training rows
- **THEN** every row records a `ctr_source` of `debiased` or `fallback`, both occur, and the model's metadata counts each

#### Scenario: Evaluation uses a temporal holdout

- **WHEN** the GBDT stage evaluates its candidate
- **THEN** the model's metadata records an evaluation protocol, a cutoff, and that every held-out impression is later than every training impression

#### Scenario: A missing ranking dataset stops the run

- **WHEN** the recsys job runs with the GBDT stage enabled and no ranking dataset
- **THEN** it exits 2, its log names RANK_DATASET_DIR, and no model is registered

#### Scenario: A disabled trainer leaves the run unchanged

- **WHEN** the recsys job runs without enabling the GBDT stage
- **THEN** the summary has no GBDT entry and the generation has no ranker artifact

### Requirement: The GBDT candidate is gated and published with its generation

The candidate SHALL be evaluated on a temporal holdout (the latest `GBDT_HOLDOUT_FRACTION` of impressions by time; training
sees none of them, including for its debiased CTR) with graded NDCG@10, and compared with the fixed-weight ranker on the same
rows. WHEN the candidate's holdout `ndcg@10` is below `baseline_ndcg@10` x 1.01 (the fixed-weight ranker on the same holdout), THE stage SHALL register it through `register_rejected` with a `gate.reason` starting `below fixed-weight baseline` and SHALL NOT evaluate the policy; otherwise the policy decides. The candidate SHALL be registered as a version of `recsys-gbdt` and evaluated by the shared gate under the `recsys-gbdt` policy (`ndcg@10` at least 0.5 % above its own champion, guard `recall@10` ratio 0.95, both inclusive), independent of the ALS champion; a candidate with no holdout with a positive SHALL be rejected, and only an engineer's `python -m modelregistry promote --force --reason` can skip the gate. The debiased CTR feature SHALL be the position-weighted
click rate of the item over the training rows excluding the row itself, used when the item has at least
`GBDT_CTR_MIN_IMPRESSIONS` other impressions, else the snapshot's `ctr_7d` (source `fallback`). A promoted candidate SHALL be
written, before the serving pointer moves, as `recs:v1:gen:<generation>:ranker`: JSON with the format `agora-gbdt/1`, the
feature list and defaults, the trees (feature index, threshold, children, leaf value), the learning rate and the metrics. A
rejected candidate SHALL leave the generation published without a ranker artifact, and the registry records it as rejected.

_Migration note (recsys-auto-train): `PROMOTION_MIN_RELATIVE_IMPROVEMENT` and `PROMOTION_FORCE` are removed (policy lives in `policies.yaml`); the old `recs:model:champion:gbdt` is seeded as v1 of `recsys-gbdt` by `migrate-registry`; the GBDT stage is on in gitops._

#### Scenario: The published artifact scores items the way the trainer does

- **WHEN** an independent evaluator reads the artifact from `recs:v1:gen:<generation>:ranker` and scores a cheap and an expensive item
- **THEN** its scores equal the trainer's to 1e-9 and the cheap item scores higher

#### Scenario: A model below the promotion threshold is rejected and the generation ships without a ranker

- **WHEN** the recsys job runs with the GBDT stage enabled and a `recsys-gbdt` champion seeded with an `ndcg@10` no model can reach
- **THEN** the GBDT candidate is recorded as rejected (`gate.outcome=rejected`, `gate.reason`), the generation is serving, and no ranker artifact exists for it

#### Scenario: RAT-62 A model below the fixed-weight baseline is structurally rejected

- **WHEN** the GBDT candidate's holdout `ndcg@10` is 0.2000 and the fixed-weight ranker scores `baseline_ndcg@10=0.2000` on the same holdout (below 0.2020 = baseline x 1.01)
- **THEN** the version is registered with `gate.outcome=rejected` through `register_rejected`, its `gate.reason` starts `below fixed-weight baseline`, the policy is not evaluated and the generation ships without a ranker artifact
- **VERIFIED BY**: `platform-recsys/tests/test_ranker_stage.py::test_below_baseline_is_structurally_rejected`
