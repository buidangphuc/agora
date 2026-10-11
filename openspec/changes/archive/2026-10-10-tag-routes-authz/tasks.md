## 1. Code — team-ai

- [x] 1.1 Tag-route auth: settings `AUTH_ADMIN_BEARER_TOKEN`/`AUTH_ADMIN_SUBJECT` with startup guards, admin-token principal, `require_tag_access` dependencies on all five routes (read: `ai.classify`|`admin`; mutate: `admin`). Verify: `tests/unit/modules/test_tag_routes_authz.py` (fails on unauthenticated code), existing tag API tests updated to send a token, `make check`, `make check-env`, `make test`.
- [x] 1.2 README contract table and known gaps; `.env.example`. Verify: `make check-env`.

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [x] 2.1 `tax_` steps/support authenticate with the service/admin tokens; four new scenarios in `ai/tag_classifier_taxonomy.feature`; FEATURES entries `planned` with note `needs rebuild`, one `not-testable`. Verify: `ruff`, `black --check`, `make -C platform-e2e features-check`.

## 3. Compose (integrator, needs rebuild)

- [x] 3.1 team-ai env of design.md in `docker-compose.services.yaml`. Verify: after rebuild the existing and new `tax_` scenarios pass.

## Evidence (2026-10-10)

- Final gate on HEAD 6b714354: parallel lane (`e2e.sh -q -n 4 -m "not destructive"`) 825/825 passed, run twice; destructive lane 103/104. The one failure, `test_c1_notification_faults::test_name_lookup_failure_still_notifies`, belongs to change C1 (not this change) and passed on isolated rerun (flaky). Stack READY.
