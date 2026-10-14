## MODIFIED Requirements

### Requirement: Degenerate candidates are rejected before the metric gate

Before comparing metrics, the job SHALL reject a candidate whose:
- share of the dataset's users with at least one recommendation is below `GATE_MIN_USER_COVERAGE`;
- share of the dataset's items that appear in any top-N list is below `GATE_MIN_ITEM_COVERAGE`;
- mean pairwise Jaccard overlap of users' top-N lists is above `GATE_MAX_LIST_OVERLAP`;
- factors contain a NaN or infinite value.

The rejection SHALL be recorded in the model registry as a version of `recsys-als` created with `register_rejected` (`gate.outcome=rejected`, `gate.reason` naming the failed
check; the metric gate is not called), and the run summary SHALL say so.

_Migration note (recsys-auto-train): the Redis `recs:model:meta:<version>` record with status `rejected` is replaced by an MLflow version tagged `gate.outcome=rejected`; no data migration, old Redis records are ignored._

#### Scenario: A one-size-fits-all model is rejected

- **WHEN** the recsys job runs on a dataset in which every user interacted with the same three items only
- **THEN** the candidate is registered as a `recsys-als` version tagged `gate.outcome=rejected` with a `gate.reason` naming the list overlap or item coverage check, and
  serving is unchanged

### Requirement: The previous generation can be restored

`python -m recsys rollback` SHALL make the previous generation the serving one: it SHALL swap `recs:v1:serving` and
`recs:v1:previous`, repoint the aliases, and, before the pointer, swap `champion` and `previous` of each registered model the serving generation carries through the model registry (registry first, then the pointer). Because serving follows the
pointer, recommendations and similar items SHALL then come from the restored generation's keys and collection. With no
previous generation it SHALL exit 2 and change nothing.

_Migration note (recsys-auto-train): `restore_champion` on the Redis registry is replaced by the registry swap (decision D6); rollback to a version with no artifact, a model without `previous`, and registry outages are covered by the ADDED requirement `Rollback follows the registry, registry first and then the pointer`._

#### Scenario: Rolling back restores the earlier model

- **WHEN** two models were promoted and `python -m recsys rollback` runs
- **THEN** `recs:v1:serving` is the first model, `recs:v1:previous` is the second, `recsys-als@champion` names the version that built the first,
  and a buyer's recommendations through the gateway report the first model's version

#### Scenario: Rollback restores the earlier model's vectors

- **WHEN** two models were promoted and `python -m recsys rollback` runs
- **THEN** similar items through the gateway are the nearest neighbours in the first model's collection

#### Scenario: Rollback without a previous generation is refused

- **WHEN** only one generation was ever promoted and `python -m recsys rollback` runs
- **THEN** it exits 2 and `recs:v1:serving` is unchanged
