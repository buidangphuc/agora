## 1. Code — team-ai

- [ ] 1.1 `TaxonomyStore` protocol + `RedisTaxonomyStore` (`tagtax:v1:*` hashes), settings `TAXONOMY_PERSISTENCE_ENABLED/REDIS_DATABASE/REDIS_PREFIX` with startup guards, `.env.example`. Verify: `tests/unit/modules/test_tag_taxonomy_persistence.py` store round-trip and guard tests (fakeredis), `make check-env`.
- [ ] 1.2 `TagClassifierService`: attach/load over the seed, persist-first `promote` (503 on failure), best-effort `explore`, lock, lazy reload after a failed startup load. Verify: the persistence tests (restart simulated by a new service on the same store; failing store), existing tag tests, `make check`, `make test`.
- [ ] 1.3 `TagTaxonomyAddon` registered in `default_resource_addons`; README (data section, known gaps, config table). Verify: addon test (enabled/disabled, REST and gRPC share the instance), `make test`.

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [ ] 2.1 `@destructive` scenario "Promoted and exploring tags survive a restart" in `ai/tag_classifier_taxonomy.feature` (restart `agora-team-ai-svc`, wait for readiness); FEATURES entry `planned` with note `needs rebuild`, four `not-testable` entries. Verify: `ruff`, `black --check`, `make -C platform-e2e features-check`.

## 3. Compose (integrator, needs rebuild)

- [ ] 3.1 team-ai env of design.md in `docker-compose.services.yaml`. Verify: after rebuild the 2.1 scenario passes in the serial lane.
