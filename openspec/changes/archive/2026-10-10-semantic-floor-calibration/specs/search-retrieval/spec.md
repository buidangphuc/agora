## MODIFIED Requirements

### Requirement: Semantic similarity floor

The system SHALL drop a semantic candidate whose cosine similarity to the query is below `HYBRID_SEMANTIC_MIN_SCORE` (default 0.65, calibrated for bge-small-en-v1.5 by `team-search/scripts/semantic_floor_probe.py`; a value of -1 or less disables it), in HYBRID and SEMANTIC mode, because k-NN has no natural cutoff and would otherwise return the k nearest listings for any text. A query whose lexical leg is empty and whose semantic candidates are all below the floor SHALL return zero hits and a zero total. OpenSearch reports `(1 + cosine) / 2` for a `cosinesimil` k-NN query on the Lucene engine; the floor is configured as a cosine and converted.

#### Scenario: Unrelated text returns no semantic candidates
- **WHEN** a buyer searches in HYBRID mode for a term that matches no listing lexically and is semantically unrelated to every listing
- **THEN** the answer is 200 with no hits and a total of zero

#### Scenario: A related listing survives the similarity floor
- **WHEN** a buyer searches in HYBRID mode for a term that matches no listing lexically but is a semantic alias of a listing's title
- **THEN** that listing is among the hits and an unrelated listing is not

#### Scenario: The default floor is calibrated
- **WHEN** team-search starts without `HYBRID_SEMANTIC_MIN_SCORE`
- **THEN** the floor is a cosine of 0.65
- **VERIFIED BY**: team-search/internal/config/config_test.go › TestSemanticMinScoreDefault. Not verifiable end to end: the e2e overlay sets its own floor for the fake TEI, so a black-box run cannot observe the default.

#### Scenario: Unrelated neighbours do not inflate total or paging
- **WHEN** a buyer searches in HYBRID mode for a keyword shared by three listings in an index holding many unrelated listings
- **THEN** the total is three and there is no next page
