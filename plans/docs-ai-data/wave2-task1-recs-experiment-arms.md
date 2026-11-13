# [W2-T1] recs-experiment-arms

## Role
feature-run worker (change `openspec/changes/recs-experiment-arms`)

## Objective
Implement OpenSpec change `recs-experiment-arms` test-first per its tasks.md; context in plans/docs-ai-data/00-spec.md.
- Optional `experiment.flag` per placement (`home_feed` -> `recs-home-feed-ranker`), Flipt assigner with salted-hash entity id and 50 ms timeout, ranker override per arm, `model_version` = `<generation>/<tier>/<ranker>/<arm>`, `recs.served` log gains tier/ranker/arm, idempotent `seed_flipt_flags.py`; compose env `RECS_FLIPT_URL`, `RECS_EXPERIMENT_SALT` (open issue 19).
- depends_on: none
- base: `asdlc/docs-ai-data` after wave 1
- hot files: team-ai/app/core/config/*.py, team-ai/app/bootstrap/services.py (ai-assistant-grounding W4, ai-copilot-grounding W5); docker-compose.services.yaml

## Write-set (EXCLUSIVE)
- team-ai/app/**
- team-ai/tests/**
- team-ai/scripts/seed_flipt_flags.py
- team-ai/.env.example
- team-ai/README.md
- team-ai/FEATURES.yaml
- docker-compose.services.yaml
- platform-e2e/tests/e2e/features/recommendations/experiment_arms.feature
- platform-e2e/tests/e2e/features/recommendations/serving_safeguards.feature
- platform-e2e/tests/e2e/features/recommendations/generation_publish.feature
- platform-e2e/tests/e2e/step_definitions/test_experiment_arms.py
- platform-e2e/tests/e2e/step_definitions/experiment_arms_steps.py
- platform-e2e/tests/e2e/step_definitions/test_rss_serving_safeguards.py
- platform-e2e/tests/e2e/step_definitions/test_rgp_generation_publish.py
- platform-e2e/tests/e2e/step_definitions/recommendations_steps.py
- platform-e2e/tests/e2e/flows/flipt_flow.py
- widened (P1):
  - platform-e2e/tests/e2e/step_definitions/era_steps.py — e2e step file of this change (experiment_arms.feature), named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/test_era_experiment_arms.py — e2e scenario binding of this change, named differently from the packet
  - platform-e2e/tests/e2e/step_definitions/rgp_steps.py — e2e steps of generation_publish.feature; the new model_version wire format `<generation>/<tier>/<ranker>/<arm>` breaks the old exact-match step
  - platform-e2e/tests/e2e/step_definitions/rss_steps.py — e2e steps of serving_safeguards.feature; fallback model_version is now prefixed, the old exact-match step breaks
  - platform-e2e/tests/e2e/support/rss_support.py — e2e support of serving_safeguards.feature; adds FALLBACK_PREFIX for the new model_version format
  - team-ai/docs/usage.md — doc updated by docs-sync (Recommend model_version format)

## Verify
- `cd team-ai && make ci` (ruff, env check, pyright, pytest)
- `openspec validate recs-experiment-arms --strict`
- `make -C platform-e2e features-check`
- e2e (serial lane, @destructive): recommendations/experiment_arms.feature, serving_safeguards.feature, generation_publish.feature
