## MODIFIED Requirements

### Requirement: Serving reads the serving generation

team-ai SHALL read recommendations from the keys of the generation named by `recs:v1:serving`, caching the pointer for at
most 5 seconds, and SHALL query Qdrant in the collection of that same generation
(`<RECS_QDRANT_COLLECTION>__<generation>`), never through the alias, so one pointer read decides both stores. The
generation SHALL be fixed once per request: the lists and the vectors of one request belong to one generation. When
`recs:v1:serving` is absent it SHALL read the unscoped v1 keys and the alias collection as before. The first segment of the
reported `model_version` (`<generation>/<tier>/<ranker>/<arm>`) SHALL be the serving generation.

#### Scenario: Serving follows a promotion within seconds

- **WHEN** a new model is promoted
- **THEN** within 10 seconds the generation segment of a buyer's `model_version` through the gateway is the new model's version
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/generation_publish.feature` `Serving follows a promotion within seconds`

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
