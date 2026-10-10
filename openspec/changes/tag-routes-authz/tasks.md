## 1. Code — team-ai

- [ ] 1.1 Tag-route auth: settings `AUTH_ADMIN_BEARER_TOKEN`/`AUTH_ADMIN_SUBJECT` with startup guards, admin-token principal, `require_tag_access` dependencies on all five routes (read: `ai.classify`|`admin`; mutate: `admin`). Verify: `tests/unit/modules/test_tag_routes_authz.py` (fails on unauthenticated code), existing tag API tests updated to send a token, `make check`, `make check-env`, `make test`.
- [ ] 1.2 README contract table and known gaps; `.env.example`. Verify: `make check-env`.

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [ ] 2.1 `tax_` steps/support authenticate with the service/admin tokens; four new scenarios in `ai/tag_classifier_taxonomy.feature`; FEATURES entries `planned` with note `needs rebuild`, one `not-testable`. Verify: `ruff`, `black --check`, `make -C platform-e2e features-check`.

## 3. Compose (integrator, needs rebuild)

- [ ] 3.1 team-ai env of design.md in `docker-compose.services.yaml`. Verify: after rebuild the existing and new `tax_` scenarios pass.
