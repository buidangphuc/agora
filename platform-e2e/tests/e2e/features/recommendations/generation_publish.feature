@recsys @recommendations @destructive
Feature: A promoted model goes live as one generation (recsys-generation-publish)
  The recsys job publishes a promoted model under keys and a Qdrant collection scoped to its
  model_version, and only then moves the `item_als_vectors` alias and `recs:v1:serving`. The previous
  generation is kept for rollback, degenerate candidates are rejected before the metric gate, and
  team-ai serves the generation `recs:v1:serving` names.

  # Every scenario here rewrites the shared serving pointer, the alias and the registry, which every
  # other recommendation scenario reads, so the whole feature is @destructive (serial lane). The
  # real recsys image runs on governed dataset fixtures that include the scenario's buyer
  # (recsys_job_flow, live mode); recommendations are read through the gateway as that buyer.

  Scenario: A second promotion keeps the first as previous
    Given a buyer is logged in
    When the recsys job promotes a model on one dataset and then promotes a better model on a second dataset
    Then `recs:v1:serving` is the second model
    And `recs:v1:previous` is the first
    And the alias points at the second model's collection
    And a buyer's recommendations through the gateway report the second model's version

  Scenario: A third promotion drops the oldest generation
    Given a buyer is logged in
    And two models have been promoted
    When a third model is promoted after the two above
    Then no key or collection of the first model remains
    And `recs:v1:previous` is the second

  Scenario: A rejected candidate leaves serving untouched
    Given a buyer is logged in
    And a model has been promoted on the good dataset
    When the recsys job runs on a dataset whose candidate fails the promotion gate
    Then `recs:v1:serving`, `recs:v1:previous` and the alias are the same as before the run

  Scenario: A one-size-fits-all model is rejected
    Given a buyer is logged in
    And a model has been promoted on the good dataset
    When the recsys job runs on a dataset in which every user interacted with the same three items only
    Then the candidate is registered as `rejected` with a reason naming the list overlap or item coverage check
    And `recs:v1:serving`, `recs:v1:previous` and the alias are the same as before the run

  Scenario: Rolling back restores the earlier model
    Given a buyer is logged in
    And two models were promoted
    When `python -m recsys rollback` runs
    Then `recs:v1:serving` is the first model
    And `recs:v1:previous` is the second
    And the registry champion is the first
    And a buyer's recommendations through the gateway report the first model's version

  Scenario: Rollback without a previous generation is refused
    Given a buyer is logged in
    And a model has been promoted on the good dataset
    When `python -m recsys rollback` runs
    Then it exits non-zero
    And `recs:v1:serving`, `recs:v1:previous` and the alias are the same as before the run

  Scenario: Serving follows a promotion within seconds
    Given a buyer is logged in
    And a model has been promoted on the good dataset and the gateway reports it
    When a new model is promoted
    Then within 10 seconds a buyer's recommendations through the gateway report the new model's version
