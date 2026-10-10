## ADDED Requirements

### Requirement: The training run SHALL evaluate the generation it produced

Every pipeline run SHALL score the model it just trained against a temporal holdout drawn from
the same interaction window, using the existing `ModelEvaluator`, and SHALL record the resulting
metrics against that run's `model_version`. A run that cannot produce metrics SHALL NOT be
treated as a promotable candidate.

#### Scenario: A run produces ranking metrics for the generation it trained

- **WHEN** the pipeline completes ALS training over a warehouse with enough interactions to form
  a holdout
- **THEN** the run reports `ndcg@10` and `coverage@10` for that generation
- **AND** the reported metrics are attributed to the run's own `model_version`, not to a
  previous run's

#### Scenario: A run with no usable holdout is not a candidate

- **WHEN** the interaction window yields no test events after the temporal split
- **THEN** the run records that no evaluation was possible
- **AND** no candidate is registered, so the promotion gate is not consulted

### Requirement: A candidate SHALL reach serving only after passing the promotion gate

The pipeline SHALL register the evaluated run as a `ModelMetadata` candidate and SHALL call the
promotion gate before any serving artifact is published. Vectors and cache entries for a rejected
candidate SHALL NOT replace the generation currently being served, and the previous generation
SHALL remain readable.

#### Scenario: A regressing candidate does not become champion

- **WHEN** a run evaluates below the incumbent champion's `ndcg@10` by more than the configured
  tolerance
- **THEN** the candidate is recorded with status `rejected`
- **AND** `recs:model:champion` still names the previous version

#### Scenario: A rejected candidate leaves the previous generation serving

- **WHEN** a candidate is rejected by the gate
- **THEN** the Qdrant points and Redis keys of the previous generation are unchanged
- **AND** `recs:v1:model_version` still names the previous generation
- **AND** no stale-generation prune has been performed

#### Scenario: The first run bootstraps the registry

- **WHEN** a run completes and no champion is registered yet
- **THEN** that run is promoted and becomes the champion
- **AND** its artifacts are published to Qdrant and Redis

### Requirement: The promotion decision SHALL be observable from the run itself

The pipeline summary SHALL report the metrics that were computed, the comparison that was made,
and the resulting decision, so that a scheduled run can be audited from its own output without
inspecting the registry.

#### Scenario: The run summary states the decision and its reason

- **WHEN** a pipeline run finishes, whether the candidate was promoted or rejected
- **THEN** the summary names the candidate `model_version`, the metric compared, the incumbent
  value, the candidate value, and the decision
- **AND** a rejected run exits without signalling failure, because rejection is a normal outcome
