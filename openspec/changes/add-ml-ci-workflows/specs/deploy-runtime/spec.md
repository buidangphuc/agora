## ADDED Requirements

### Requirement: Automated CI pipelines for ML repositories

The system SHALL provide GitHub Actions workflows for `platform-recsys`, `team-ai`, and `platform-modelserve` that run automated validation checks on pull requests and pushes to `main`.

#### Scenario: platform-recsys CI executes quality gates

- **WHEN** a pull request modifies `platform-recsys`
- **THEN** the workflow runs linting, syntax compilation, environment drift verification, and offline evaluation tests

#### Scenario: platform-modelserve CI validates contract conformance

- **WHEN** a pull request modifies `platform-modelserve`
- **THEN** the workflow executes unit tests and contract conformance checks against the `_extract_vectors` schema
