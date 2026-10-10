## 1. Code — team-ai

- [ ] 1.1 `ShoppingAssistant` retrieves from RAG, builds distinct bounded cards, fails open without fabricating; add `ASSISTANT_RAG_MIN_SCORE` to settings and `.env.example`. Verify: `tests/unit/modules/test_ai_assistant_rag.py` (fails on the old catalog-only code), `make check`, `make test`.
- [ ] 1.2 README: contract/behaviour text (remove "never calls the injected RAG service"), env and deploy notes. Verify: `make check-env`.

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [ ] 2.1 `ai/assistant_grounding.feature` with the three e2e scenarios (the third `@destructive`), steps `agr_steps.py`, helper `agr_support.py`; FEATURES entries `planned` with note `needs rebuild`, the two unit-only scenarios `not-testable`. Verify: `make -C platform-e2e features-check`, `ruff`, `black --check`.

## 3. Compose (integrator, needs rebuild)

- [ ] 3.1 Add the team-ai block of design.md to `platform-e2e/compose/modelserve.override.yaml` and the `UV_EXTRAS` build arg. Verify: `docker compose config` renders; after rebuild the 2.1 scenarios pass.
