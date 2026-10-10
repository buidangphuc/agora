# recsys-generations Specification

## Purpose
Defines how a promoted recommendation model becomes the serving generation atomically, how the previous generation
is kept and restored, which degenerate candidates are rejected, and how serving follows the generation pointer.

## Requirements

### Requirement: A promoted model goes live as one generation, atomically

When the recsys job promotes a model, it SHALL write that model's recommendations under keys scoped to its
`model_version` and its item vectors to a collection named for it. Only after all of them are written SHALL it point
`recs:v1:serving` at the new generation and set `recs:v1:previous` to the generation it replaced; the pointer is the
serving decision for both stores. It SHALL also move the `item_als_vectors` / `user_als_vectors` aliases to the new
generation's collections, as a compatibility shim for readers that predate pointer-resolved collections (deprecated,
removable in a later release). `recs:v1:model_version` SHALL equal `recs:v1:serving`. Generations other than the serving
and previous ones SHALL be deleted. A rejected candidate SHALL change none of the serving data.

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
`recs:v1:previous`, repoint the aliases, and set the registry champion to the restored model. Because serving follows the
pointer, recommendations and similar items SHALL then come from the restored generation's keys and collection. With no
previous generation it SHALL exit non-zero and change nothing.

#### Scenario: Rolling back restores the earlier model

- **WHEN** two models were promoted and `python -m recsys rollback` runs
- **THEN** `recs:v1:serving` is the first model, `recs:v1:previous` is the second, the registry champion is the first,
  and a buyer's recommendations through the gateway report the first model's version

#### Scenario: Rollback restores the earlier model's vectors

- **WHEN** two models were promoted and `python -m recsys rollback` runs
- **THEN** similar items through the gateway are the nearest neighbours in the first model's collection

#### Scenario: Rollback without a previous generation is refused

- **WHEN** only one generation was ever promoted and `python -m recsys rollback` runs
- **THEN** it exits non-zero and `recs:v1:serving` is unchanged

### Requirement: Serving reads the serving generation

team-ai SHALL read recommendations from the keys of the generation named by `recs:v1:serving`, caching the pointer for at
most 5 seconds, and SHALL query Qdrant in the collection of that same generation
(`<RECS_QDRANT_COLLECTION>__<generation>`), never through the alias, so one pointer read decides both stores. The
generation SHALL be fixed once per request: the lists and the vectors of one request belong to one generation. When
`recs:v1:serving` is absent it SHALL read the unscoped v1 keys and the alias collection as before. The reported
`model_version` SHALL be the serving generation.

#### Scenario: Serving follows a promotion within seconds

- **WHEN** a new model is promoted
- **THEN** within 10 seconds a buyer's recommendations through the gateway report the new model's version

#### Scenario: A similar-items request reads the vectors of the serving generation

- **WHEN** two models have been promoted and a buyer opens similar items for a listing
- **THEN** the returned listings are the nearest neighbours in the second model's collection

#### Scenario: An alias ahead of the pointer does not change what is served

- **WHEN** the item alias is moved to the second model's collection while `recs:v1:serving` still names the first
  (a publish that crashed between the two switches)
- **THEN** similar items through the gateway are the nearest neighbours in the first model's collection

#### Scenario: An alias behind the pointer does not change what is served

- **WHEN** `recs:v1:serving` names the second model while the item alias still points at the first model's collection
- **THEN** similar items through the gateway are the nearest neighbours in the second model's collection

#### Scenario: Without a pointer the alias is used

- **WHEN** `recs:v1:serving` is absent and the item alias points at a generation's collection
- **THEN** similar items through the gateway are the nearest neighbours in the alias's collection

### Requirement: Retention never deletes a generation that serving or previous names

Deleting old generations SHALL spare every key set and Qdrant collection of the generations named by `recs:v1:serving`
and `recs:v1:previous`, whether or not an alias points at them, and SHALL delete nothing when neither pointer can be
read.

#### Scenario: Serving and previous collections survive retention without an alias

- **WHEN** a third model is promoted and the item alias is then deleted and another model is promoted
- **THEN** the collections of the serving and the previous model both still exist

### Requirement: A ranker artifact belongs to its generation

A ranker artifact written by the run SHALL be scoped to the generation (`recs:v1:gen:<generation>:ranker`), written before the
serving pointer moves with the same TTL as the generation's other keys, refreshed with them, and deleted with them by
retention: only the serving and previous generations keep theirs.

#### Scenario: The ranker artifact lives and dies with its generation

- **WHEN** the recsys job promotes three generations in a row with the GBDT stage enabled
- **THEN** a ranker artifact with a TTL exists for the serving and previous generations and none for the oldest
