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
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_evaluator_owns_temporal_split_and_reports_metadata. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

#### Scenario: Split admits no temporal leakage

- **WHEN** the temporal split is applied to an interaction fixture
- **THEN** every training event occurs at or before the cutoff
- **AND** every holdout event occurs after the cutoff
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_temporal_split_zero_leakage. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

#### Scenario: Post-cutoff-only signal is not learnable from the training half

- **WHEN** a fixture places a user's entire affinity for one item after the cutoff, and a model is
  trained on the training half only
- **THEN** that item's recall on the holdout is near zero
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_post_cutoff_only_signal_yields_zero_recall_on_training_model. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.

### Requirement: Externally supplied ground truth is labelled

When the caller supplies a pre-split ground truth, the report SHALL label the strategy as
external so reports produced under different splits are not silently compared.

#### Scenario: External split is marked in the report

- **WHEN** evaluation runs against caller-supplied ground-truth sets
- **THEN** `split_strategy` in the report is `"external"`
- **VERIFIED BY**: platform-recsys/tests/test_evals_temporal_cli.py › test_external_ground_truth_is_marked_external. Not verifiable end to end: offline evaluation logic over a fixture with no edge-visible effect; the pipeline's use of it is covered by the archived wire-pipeline-eval-registry e2e.
