## ADDED Requirements

### Requirement: Model registry metadata and promotion gate

The system SHALL provide a model registry and champion/challenger promotion gate in `platform-recsys/recsys/registry/` that evaluates newly trained candidate models against current champion models and promotes candidates only when metric thresholds are met.

#### Scenario: Candidate model passes promotion gate and becomes champion

- **WHEN** a candidate model is evaluated and achieves `ndcg@10 >= champion ndcg@10` while maintaining `catalog_coverage >= 0.8 * champion coverage`
- **THEN** the model registry promotes the candidate, sets its status to `champion`, and atomically updates `recs:model:champion` to the new version

#### Scenario: Regressed candidate model is rejected

- **WHEN** a candidate model exhibits lower ranking metrics than the current champion
- **THEN** the promotion gate rejects the candidate, its status is marked `rejected`, and the existing champion remains active without disruption
