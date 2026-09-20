## ADDED Requirements

### Requirement: Offline evaluation owns its train/test split

The offline evaluation entrypoint SHALL derive the train and holdout sets from raw interactions
using a time-based split, rather than accepting a caller-prepared ground truth as its only input.
The report SHALL record the cutoff timestamp, the train and test event counts, and the split
strategy used.

#### Scenario: Evaluation report exposes the split it performed

- **WHEN** the evaluation entrypoint runs over a raw interaction fixture
- **THEN** the report contains `cutoff_timestamp`, `train_events`, `test_events` and
  `split_strategy`
- **AND** the metrics are computed against the holdout produced by that split

#### Scenario: Split admits no temporal leakage

- **WHEN** the temporal split is applied to an interaction fixture
- **THEN** every training event occurs at or before the cutoff
- **AND** every holdout event occurs after the cutoff

#### Scenario: Post-cutoff-only signal is not learnable from the training half

- **WHEN** a fixture places a user's entire affinity for one item after the cutoff, and a model is
  trained on the training half only
- **THEN** that item's recall on the holdout is near zero

### Requirement: Externally supplied ground truth is labelled

When the caller supplies a pre-split ground truth, the report SHALL label the strategy as
external so reports produced under different splits are not silently compared.

#### Scenario: External split is marked in the report

- **WHEN** evaluation runs against caller-supplied ground-truth sets
- **THEN** `split_strategy` in the report is `"external"`
