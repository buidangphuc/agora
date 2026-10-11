## 1. Code — compose + docs

- [x] 1.1 Base `docker-compose.services.yaml`: add `tei-fake`, `modelserve-router`; team-ai RAG env, `"recs ai kafka"` extras, `depends_on`. Verify: `docker compose config -q`; team-ai up healthy with `RAG_ENABLED=true`.
- [x] 1.2 `modelserve.override.yaml` reduced to test-only settings; new `rag-off.override.yaml`; `platform-e2e/compose/README.md`. Verify: `docker compose config -q` with each overlay and with the stack wrapper.
- [x] 1.3 `team-ai/README.md` env table, compose section, gotchas. Verify: `make check-env` unaffected (no Settings change).

## 2. E2E — platform-e2e + team-ai/FEATURES.yaml

- [x] 2.1 Scenario "Without a RAG store the demo catalog answers" (`@destructive`, `agr_steps.py`); FEATURES `ai.arg-demo-catalog-without-rag` -> `automated`. Verify: `features.py --strict`, `ruff`/`black`, then `e2e.sh -q -k "assistant_grounding"` against the recreated team-ai.
