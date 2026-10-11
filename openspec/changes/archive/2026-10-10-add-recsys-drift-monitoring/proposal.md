# Proposal: add-recsys-drift-monitoring

## Summary
Implements automated feature and prediction distribution drift monitoring in `platform-recsys` using Population Stability Index (PSI) and statistical divergence metrics to detect data drift, concept drift, and model decay before production performance degrades.

## Motivation
Changes in consumer catalog trends, seasonal purchasing behavior, or upstream schema changes can lead to feature distribution shifts. A model trained on stale data will silently degrade unless automated drift detection continuously compares production inference distributions against training baselines.

## Architecture
- `recsys.monitoring.drift.calculate_psi`: calculates Population Stability Index across binned distributions.
- `recsys.monitoring.drift.DriftDetector`: compares baseline reference distributions with current window distributions for numerical and categorical features.
- Flags drift severity (`NO_DRIFT` < 0.1, `MODERATE_DRIFT` 0.1–0.25, `SIGNIFICANT_DRIFT` > 0.25).
- Generates Prometheus metric payloads and health status reports.
