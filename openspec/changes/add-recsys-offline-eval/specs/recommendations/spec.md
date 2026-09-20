## ADDED Requirements

### Requirement: Offline recommendation evaluation harness

The system SHALL provide an offline evaluation metrics suite in `platform-recsys/recsys/evals/` that computes NDCG@K, Recall@K, Precision@K, MAP@K, MRR@K, HitRate@K, and Catalog Coverage over ground truth holdout datasets.

#### Scenario: Evaluator computes ranking metrics for Top-K recommendations

- **WHEN** the evaluator is run with a ground truth dictionary of user interactions and a model's predicted Top-K recommendations
- **THEN** it outputs a summary dictionary containing `ndcg@10`, `recall@10`, `precision@10`, `map@10`, `mrr@10`, `hit_rate@10`, and `catalog_coverage` with values bounded between 0.0 and 1.0

#### Scenario: Perfect prediction yields maximum metrics

- **WHEN** the predicted ranking matches the actual ground truth relevant items exactly in order
- **THEN** `ndcg@k`, `recall@k`, `precision@k`, `map@k`, `mrr@k`, and `hit_rate@k` all equal 1.0

#### Scenario: Empty or disjoint predictions yield zero metrics

- **WHEN** none of the predicted items exist in the actual ground truth set for a user
- **THEN** `ndcg@k`, `recall@k`, `precision@k`, `map@k`, `mrr@k`, and `hit_rate@k` all equal 0.0
