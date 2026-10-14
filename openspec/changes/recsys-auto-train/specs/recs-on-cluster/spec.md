## MODIFIED Requirements

### Requirement: Recommendation pods reach only the stores they need

The recsys CronJob pods and the nearline pods SHALL each have a NetworkPolicy with no ingress and egress limited to DNS and
their stores: recsys to `qdrant`, `valkey` and `mlflow` (TCP 5050), nearline to `redpanda` and `valkey`.

_Migration note (recsys-auto-train): the recsys egress gains the model registry (MLflow, cluster-internal); nothing else changes._

#### Scenario: The recsys job and nearline egress is limited to their stores

- **WHEN** the recsys and nearline NetworkPolicies are rendered
- **THEN** each allows no ingress, and its egress targets are DNS plus exactly the store pods and ports named above
