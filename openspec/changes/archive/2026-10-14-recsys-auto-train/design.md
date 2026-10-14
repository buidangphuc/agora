# Design notes: recsys-auto-train

## Clarifications

### 2026-10-13 (human-approved in chat)

1. **Two-tower policy has no coverage floor.** The `coverage@10` floor of 0.5 is unreachable on a real catalogue (measured coverage about 0.0009). The `recsys-two-tower` policy is `{primary: recall@10, min_relative_improvement: 0.005, guards: {coverage@10: 0.8}, floors: {recall@10: 0.000001}}`; the first champion needs `recall@10` > 0. Affects RAT-12, RAT-41, the GBDT/two-tower requirement, tasks 1.1 and 2.7.
2. **GBDT keeps the fixed-weight baseline check.** WHEN the candidate's holdout `ndcg@10` < `baseline_ndcg@10` x 1.01 (fixed-weight ranker, same holdout), the stage registers it via `register_rejected` with `gate.reason` starting `below fixed-weight baseline` and does not evaluate the policy; otherwise the policy decides. Scenario RAT-62, task 1.5.
3. **Drift observability: the second run is promoted.** The behaviour stays "the second run is promoted" (not "decided by the gate"). The second run uses a dataset on which the model genuinely improves by at least the ALS policy threshold, so it promotes and drift is computed against the first run's champion sketches. Task 2.11.
