## ADDED Requirements

### Requirement: A promoted model goes live as one generation, atomically

When the recsys job promotes a model, it SHALL write that model's recommendations under keys scoped to its
`model_version` and its item vectors to a collection named for it. Only after all of them are written SHALL it point
the `item_als_vectors` alias and `recs:v1:serving` at the new generation, and set `recs:v1:previous` to the generation
it replaced. `recs:v1:model_version` SHALL equal `recs:v1:serving`. Generations other than the serving and previous ones
SHALL be deleted. A rejected candidate SHALL change none of the serving data.

#### Scenario: A second promotion keeps the first as previous

- **WHEN** the recsys job promotes a model on one dataset and then promotes a better model on a second dataset
- **THEN** `recs:v1:serving` is the second model, `recs:v1:previous` is the first, the alias points at the second
  model's collection, and a buyer's recommendations through the gateway report the second model's version

#### Scenario: A third promotion drops the oldest generation

- **WHEN** a third model is promoted after the two above
- **THEN** no key or collection of the first model remains, and `recs:v1:previous` is the second model

#### Scenario: A rejected candidate leaves serving untouched

- **WHEN** the recsys job runs on a dataset whose candidate fails the promotion gate
- **THEN** `recs:v1:serving`, `recs:v1:previous` and the alias are the same as before the run

### Requirement: Degenerate candidates are rejected before the metric gate

Before comparing metrics, the job SHALL reject a candidate whose:
- share of the dataset's users with at least one recommendation is below `GATE_MIN_USER_COVERAGE`;
- share of the dataset's items that appear in any top-N list is below `GATE_MIN_ITEM_COVERAGE`;
- mean pairwise Jaccard overlap of users' top-N lists is above `GATE_MAX_LIST_OVERLAP`;
- factors contain a NaN or infinite value.

The rejection SHALL be recorded on the model in the registry with status `rejected` and a reason naming the failed
check, and the run summary SHALL say so.

#### Scenario: A one-size-fits-all model is rejected

- **WHEN** the recsys job runs on a dataset in which every user interacted with the same three items only
- **THEN** the candidate is registered as `rejected` with a reason naming the list overlap or item coverage check, and
  serving is unchanged

### Requirement: The previous generation can be restored

`python -m recsys rollback` SHALL make the previous generation the serving one: it SHALL swap `recs:v1:serving` and
`recs:v1:previous`, repoint the alias, and set the registry champion to the restored model. With no previous
generation it SHALL exit non-zero and change nothing.

#### Scenario: Rolling back restores the earlier model

- **WHEN** two models were promoted and `python -m recsys rollback` runs
- **THEN** `recs:v1:serving` is the first model, `recs:v1:previous` is the second, the registry champion is the first,
  and a buyer's recommendations through the gateway report the first model's version

#### Scenario: Rollback without a previous generation is refused

- **WHEN** only one generation was ever promoted and `python -m recsys rollback` runs
- **THEN** it exits non-zero and `recs:v1:serving` is unchanged

### Requirement: Serving reads the serving generation

team-ai SHALL read recommendations from the keys of the generation named by `recs:v1:serving`, caching the pointer for at
most 5 seconds. When `recs:v1:serving` is absent it SHALL read the unscoped v1 keys as before. The reported
`model_version` SHALL be the serving generation.

#### Scenario: Serving follows a promotion within seconds

- **WHEN** a new model is promoted
- **THEN** within 10 seconds a buyer's recommendations through the gateway report the new model's version
