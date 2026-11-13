## MODIFIED Requirements

### Requirement: Recommendations degrade instead of failing

When reading the cache or Qdrant fails, `Recommend` SHALL still answer `OK`. It SHALL return the serving generation's
popular list, or an empty list if that cannot be read either, with a `model_version` whose generation and tier segments are
`serving-fallback` (`serving-fallback/serving-fallback/similarity/<arm>`). It SHALL answer
`UNAVAILABLE` only when the vector collection does not match the producer's contract (a deployment error, not an
outage). Recommendations have no on/off flag: they are always served.

#### Scenario: Recommendations survive a Redis outage

- **WHEN** the Redis used by team-ai is stopped and a buyer requests homepage recommendations through the gateway
- **THEN** the call succeeds with a `model_version` that starts with `serving-fallback/serving-fallback/`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/recommendations/serving_safeguards.feature` `Recommendations survive a Redis outage`
