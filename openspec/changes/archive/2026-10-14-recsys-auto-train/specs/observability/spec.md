## MODIFIED Requirements

### Requirement: Each run is compared with the generation it replaces
Every recsys run SHALL summarise its training distribution (the dataset's pair weights, items per user, users per item)
and its predictions (each user's best recommendation score) and store that summary as the JSON artifact `drift_sketches.json` of its MLflow run. It SHALL
compute the PSI of those features against the `drift_sketches.json` of the run behind the `champion` version of `recsys-als`, which is the generation the run
would replace, and record the verdict as params of its MLflow run (`drift.*`, with the baseline version, the
alert threshold, per-feature PSI and level, and an overall flag; the largest PSI as the metric `drift_psi_max`) and in the
run summary. A feature is flagged when its PSI reaches `DRIFT_ALERT_THRESHOLD` (0.25). The verdict SHALL NOT reject,
delay or otherwise change the promotion decision. A run with no champion, or whose champion has no stored summary, SHALL
record the status `no_baseline`. The gate-independent comparison SHALL NOT use the ALS factors themselves, whose values are
only defined up to a rotation. Where a second run is compared with the first, the second run uses a dataset on which the model genuinely improves by at least the `recsys-als` policy threshold (`ndcg@10` +1 %), so it is promoted and its drift is computed against the first run's champion sketches.

_Migration note (recsys-auto-train): the baseline moves from `recs:model:meta:<version>` `parameters.distribution` to the champion run's `drift_sketches.json`; a seeded v1 has no sketches, so the first run after migration reports `no_baseline`._

#### Scenario: The first run has no drift baseline

- **WHEN** the recsys job runs with no `champion` for `recsys-als`
- **THEN** the run summary and the run's params report the drift status `no_baseline`, and the model stores its
  distribution summary for the next run

#### Scenario: A second run records its drift against the first

- **WHEN** the recsys job runs a second time on a governed dataset on which the model improves `ndcg@10` by at least 1 % over the first run
- **THEN** the second run is promoted, its drift names the first run (the champion it replaces) as its baseline using that run's `drift_sketches.json`, reports a PSI and a level for each of
  `weight`, `user_items`, `item_users` and `top_score`, and flags none of them

#### Scenario: A drifted run is flagged and still decided by the gate

- **WHEN** the recsys job runs a second time with `DRIFT_ALERT_THRESHOLD` set to 0
- **THEN** the second run's drift is flagged for every feature, its run params carry the same verdict, and its promotion
  decision is the one the gate gives
