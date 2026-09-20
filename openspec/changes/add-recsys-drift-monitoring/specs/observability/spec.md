# Capability: RecSys Drift Monitoring

## ADDED Requirements

### Requirement: Population Stability Index Calculation
The system MUST calculate PSI for numerical and categorical feature distributions between a baseline dataset and a target dataset.

#### Scenario: Identical distributions have zero drift
- **GIVEN** a baseline feature sample and an identical target feature sample
- **WHEN** PSI is computed
- **THEN** the PSI score is approximately 0.0 and drift status is NO_DRIFT.

#### Scenario: Shifted distribution triggers significant drift alert
- **GIVEN** a baseline distribution centered at one range and a target distribution shifted significantly
- **WHEN** PSI is computed
- **THEN** the PSI score exceeds 0.25 and drift status is SIGNIFICANT_DRIFT.

### Requirement: Multi-Feature Drift Report
The system MUST generate a comprehensive drift assessment across multiple features, reporting individual PSI metrics and an overall model drift flag.

#### Scenario: Multi-feature dataset drift evaluation
- **GIVEN** a dictionary of baseline and target feature columns
- **WHEN** drift evaluation is run
- **THEN** a structured report containing per-feature PSI, drift levels, and whether any feature exceeds the alert threshold is produced.
