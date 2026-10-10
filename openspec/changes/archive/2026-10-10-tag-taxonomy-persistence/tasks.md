## 1. Code — team-ai

- [x] 1.1 `TaxonomyStore` protocol + `RedisTaxonomyStore` (`tagtax:v1:*` hashes), settings `TAXONOMY_PERSISTENCE_ENABLED/REDIS_DATABASE/REDIS_PREFIX` with startup guards, `.env.example`. Verify: `tests/unit/modules/test_tag_taxonomy_persistence.py` store round-trip and guard tests (fakeredis), `make check-env`.
- [x] 1.2 `TagClassifierService`: attach/load over the seed, persist-first `promote` (503 on failure), best-effort `explore`, lock, lazy reload after a failed startup load. Verify: the persistence tests (restart simulated by a new service on the same store; failing store), existing tag tests, `make check`, `make test`.
- [x] 1.3 `TagTaxonomyAddon` registered in `default_resource_addons`; README (data section, known gaps, config table). Verify: addon test (enabled/disabled, REST and gRPC share the instance), `make test`.

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [x] 2.1 `@destructive` scenario "Promoted and exploring tags survive a restart" in `ai/tag_classifier_taxonomy.feature` (restart `agora-team-ai-svc`, wait for readiness); FEATURES entry `planned` with note `needs rebuild`, four `not-testable` entries. Verify: `ruff`, `black --check`, `make -C platform-e2e features-check`.

## 3. Compose (integrator, needs rebuild)

- [x] 3.1 team-ai env of design.md in `docker-compose.services.yaml`. Verify: after rebuild the 2.1 scenario passes in the serial lane.

## Evidence (2026-10-10)

- Final gate on HEAD 6b714354: parallel lane (`e2e.sh -q -n 4 -m "not destructive"`) 825/825 passed, run twice; destructive lane 103/104. The one failure, `test_c1_notification_faults::test_name_lookup_failure_still_notifies`, belongs to change C1 (not this change) and passed on isolated rerun (flaky). Stack READY.
