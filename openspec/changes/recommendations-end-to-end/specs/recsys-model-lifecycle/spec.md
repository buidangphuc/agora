## Purpose

Defines the life of a recommendation model from a scheduled training run to the moment serving uses it:
how a run is triggered, how a candidate is evaluated and either promoted or refused, how its vectors and
lists become visible all at once, how the previous model stays available for rollback, and what each run
reports.

## ADDED Requirements

### Requirement: Training runs on a schedule in every deployed environment

The training job SHALL run on a schedule in every deployed environment against the latest complete
dataset build of its feature view and SHALL reach the vector store, the cache and the object storage that
serving and the feature store actually use in that environment. Two runs SHALL never overlap. The same job SHALL be runnable on
demand (locally and in-cluster) with the same configuration.

#### Scenario: A scheduled run produces a new generation

- **WHEN** the schedule fires and a fresh, valid dataset build of the view exists
- **THEN** a run starts, trains on that build, and ends with a promoted generation whose model version
  is reported by `Recommend`

#### Scenario: Runs never overlap

- **WHEN** a run is still in progress when the schedule fires again
- **THEN** the second run does not start, and the first run completes unaffected

### Requirement: A candidate model is evaluated on a time-based holdout

Each run SHALL evaluate its candidate on a time-based holdout (training on the view's point-in-time
snapshot at the holdout start, evaluating on the interactions that first appear between the holdout start
and the build's watermark; never a random split) and SHALL compute at least Recall@K, NDCG@K,
catalog coverage and the same Recall@K for the popularity baseline over the same holdout. The evaluation
report SHALL be stamped with the model version, the dataset build id and the window boundaries, and SHALL be
retained alongside the run.

#### Scenario: The report carries metrics and provenance

- **WHEN** a run finishes evaluation
- **THEN** its report holds Recall@K, NDCG@K, coverage, the popularity baseline's Recall@K, the model
  version, the dataset build id and the train/holdout boundaries

### Requirement: A promotion gate refuses bad or degenerate models

A candidate SHALL be promoted only if every gate check passes: Recall@K is at least the configured minimum,
Recall@K is at least the popularity baseline multiplied by the configured minimum lift, the model has at
least the configured minimum number of users and items, every vector is finite, catalog coverage is at
least the configured minimum, and the mean overlap between different users' lists is at most the
configured maximum (a model that gives everyone the same list is degenerate). A refused candidate SHALL
NOT be visible to serving in any way, and the run SHALL record which check failed.

#### Scenario: A model below the minimum metric is refused

- **WHEN** a candidate's holdout Recall@K is below the configured minimum
- **THEN** the run is recorded as refused with that check named, and `Recommend` keeps returning the
  previous model version and the previous lists

#### Scenario: An empty model is refused

- **WHEN** training produces no user factors or fewer items than the configured minimum
- **THEN** the candidate is refused as degenerate and nothing of it is written where serving reads

#### Scenario: A non-personalized model is refused

- **WHEN** a candidate gives different users lists whose mean overlap exceeds the configured maximum
- **THEN** the candidate is refused as degenerate with the overlap check named

#### Scenario: A model that loses to popularity is refused

- **WHEN** a candidate's Recall@K is below the popularity baseline's Recall@K times the minimum lift
- **THEN** the candidate is refused with the lift check named

#### Scenario: A first run with no previous model still provides a floor

- **WHEN** a candidate is refused and no generation has ever been promoted
- **THEN** a popularity-only generation (popular list, no personal lists, no vectors) is published so
  serving has a floor, and the run is still recorded as refused for the model

### Requirement: A generation becomes visible atomically

A promoted generation's item vectors, per-user lists, per-item lists and popular list SHALL become visible
to serving in a single step that happens only after every artifact of the generation has been written and
verified (counts read back match what was written). Until that step, serving SHALL see only the previous
generation; after it, only the new one. Serving SHALL never combine artifacts from two generations in one
response. A run that fails at any point before the step SHALL leave the previous generation fully
served.

#### Scenario: The version flips only after a successful load

- **WHEN** a run writes the new generation's lists and vectors and then verifies them
- **THEN** the served model version changes only after the verification succeeds, and every list and
  vector served after the change belongs to the new generation

#### Scenario: A crash mid-load leaves the old model served

- **WHEN** a run is killed after writing part of the new generation's lists or vectors
- **THEN** `Recommend` keeps returning the previous model version with its complete lists, and no user
  receives a list from the half-written generation

#### Scenario: Users absent from the new model do not keep stale personal lists

- **WHEN** a user had a personal list in the previous generation and is not part of the new generation
- **THEN** after the flip that user is served as a cold-start user, not the previous generation's list

### Requirement: The previous generation is kept and can be restored

The system SHALL keep at least the previous promoted generation intact after a flip, and SHALL provide a
rollback operation that makes the previous generation current again in one step, without retraining. Older
generations beyond the kept ones SHALL be removed after a successful flip. A rollback SHALL be recorded
like a run.

#### Scenario: Rollback restores the previous model

- **WHEN** an operator runs the rollback operation after generation B replaced generation A
- **THEN** within the serving version-cache interval `Recommend` reports A's model version and returns A's
  lists and A's similar items, and generation B is still kept so it can be restored again

#### Scenario: Rollback with nothing to roll back to is refused

- **WHEN** an operator runs rollback while only one generation exists
- **THEN** the operation fails with a clear message and the current generation is unchanged

#### Scenario: Old generations are cleaned up

- **WHEN** a third generation C is promoted after A and B
- **THEN** C is current, B is kept as the rollback target, and A's lists and vectors no longer exist

### Requirement: A new model family can be published as a challenger

The job SHALL support a challenger publish mode in which a candidate that passed the offline gate becomes the
challenger generation instead of the current one, leaving the current and previous generations untouched.
Operators SHALL be able to start serving the challenger to a share of users (at most 50 percent), stop it,
or promote it to current, each in one step. Generation cleanup SHALL never delete the current, previous or
challenger generation.

#### Scenario: A challenger publish does not change what most users see

- **WHEN** a run in challenger mode passes the gate while a current generation exists
- **THEN** the challenger pointer names the new generation, the current pointer is unchanged, and with
  share 0 every `Recommend` response still carries the current model version

#### Scenario: The challenger survives nightly cleanup

- **WHEN** a nightly run promotes a new current generation while a challenger is set
- **THEN** the challenger generation's lists and vectors still exist and are still served to its bucket

### Requirement: Each run reports its outcome

Every run, rollback and challenger action SHALL record a report readable by serving and operators: status
(promoted, refused, failed, skipped, rolled back, challenger set, challenger promoted, challenger stopped),
reason, model version, dataset build id and watermark age, evaluation metrics, item/user counts and
start/finish times. The last report SHALL be kept even when the run did not promote.

#### Scenario: A failed run is visible

- **WHEN** a run fails because the vector store is unreachable
- **THEN** the last-run report says failed with that reason and the served generation is unchanged

### Requirement: The full pipeline runs locally on a tiny dataset

A developer SHALL be able to run, from the workspace root with the local stack up, the real pipeline end to
end on a small generated dataset: generate behavioural events through the gateway, materialize the feature
view's dataset in the feature store, train (Spark local mode), evaluate, gate and publish, and then see the result through `Recommend`. No step SHALL
rely on hand-written Redis keys or Qdrant points, and the gate thresholds for the local environment SHALL
be set so a healthy tiny dataset passes.

#### Scenario: A developer runs real training locally

- **WHEN** a developer runs the documented local pipeline command on a fresh stack
- **THEN** it finishes with a promoted generation, and a logged-in buyer who took part in the generated
  traffic gets a `Recommend` response carrying that generation's model version
