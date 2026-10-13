## MODIFIED Requirements

### Requirement: The GBDT candidate is gated and published with its generation

The candidate SHALL be evaluated on a temporal holdout (the latest `GBDT_HOLDOUT_FRACTION` of impressions by time; training
sees none of them, including for its debiased CTR) with graded NDCG@10, and compared with the fixed-weight ranker on the same
rows. The candidate SHALL be registered as a version of `recsys-gbdt` and evaluated by the shared gate under the `recsys-gbdt` policy (`ndcg@10` at least 0.5 % above its own champion, guard `recall@10` ratio 0.95, both inclusive), independent of the ALS champion; a candidate with no holdout with a positive SHALL be rejected, and only an engineer's `python -m modelregistry promote --force --reason` can skip the gate. The debiased CTR feature SHALL be the position-weighted
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
