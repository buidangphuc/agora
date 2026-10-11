## ADDED Requirements

### Requirement: A ranker artifact belongs to its generation

A ranker artifact written by the run SHALL be scoped to the generation (`recs:v1:gen:<generation>:ranker`), written before the
serving pointer moves with the same TTL as the generation's other keys, refreshed with them, and deleted with them by
retention: only the serving and previous generations keep theirs.

#### Scenario: The ranker artifact lives and dies with its generation

- **WHEN** the recsys job promotes three generations in a row with the GBDT stage enabled
- **THEN** a ranker artifact with a TTL exists for the serving and previous generations and none for the oldest
