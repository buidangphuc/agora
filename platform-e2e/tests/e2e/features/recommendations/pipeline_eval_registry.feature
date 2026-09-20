@recsys @batch @registry
Feature: Pipeline Evaluation, Model Registry, and Promotion Gate (wire-pipeline-eval-registry)
  As the Agora RecSys platform, offline training runs must evaluate the candidate
  generation against a temporal holdout, gate promotion against the incumbent
  champion, and only publish artifacts to Qdrant/Redis if promoted.

  Scenario: A run produces ranking metrics for the generation it trained
    Given an offline batch pipeline run over warehouse tracking events
    When ALS training completes and the temporal holdout is scored
    Then the run reports ndcg@10 and coverage@10 metrics attributed to the run's model_version

  Scenario: A run with no usable holdout is not a candidate
    Given an interaction window with no test events after temporal split
    When the evaluation stage runs
    Then no candidate is registered and the promotion gate is skipped

  Scenario: A regressing candidate does not become champion
    Given an incumbent champion model in the registry
    When a candidate model evaluates below the incumbent champion tolerance
    Then the candidate status is recorded as rejected
    And the champion key still names the previous version

  Scenario: A rejected candidate leaves the previous generation serving
    Given a candidate model rejected by the promotion gate
    When the pipeline run finishes
    Then serving vector collections and active model_version remain on the previous generation

  Scenario: The first run bootstraps the registry
    Given an empty model registry with no existing champion
    When the initial pipeline run completes with valid metrics
    Then the initial model is promoted as champion and published to serving stores

  Scenario: The run summary states the decision and its reason
    Given an offline pipeline execution
    When the pipeline completes evaluation and gating
    Then the summary reports model_version, metrics, decision, and promotion reason
