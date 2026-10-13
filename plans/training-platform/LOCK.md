# LOCK — training-platform
status: approved
approved-by: Bùi Đăng phúc
date: 2026-10-13
spec: plans/training-platform/00-spec.md (Locked 2026-10-13)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| add-model-registry | platform-core | [] | 3 (r3 = non-blocking polish) | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 28d7497e15e2 | [x] |
| recsys-auto-train | platform-recsys | [add-model-registry] | 3 (r3 = non-blocking polish) | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 36bb6a0c1a64 (re-approved 2026-10-13: clarifications) | [x] |
| add-platform-finetune | platform-finetune | [add-model-registry, recsys-auto-train] | 3 | pass | pass | pass (r3) | ENGINE_DOWN (LLM fallback) | 0b738ab054d9 | [x] |

## Build order (cross_change_check)
1. add-model-registry (sequential, provides the py-modelregistry API)
2. recsys-auto-train
3. add-platform-finetune

Hot-file pairs that must not run in parallel: recsys-auto-train and add-platform-finetune share
`docker-compose.services.yaml` and `platform-core/packages/py-modelregistry/policies.yaml`; add-model-registry and
add-platform-finetune share `.github/workflows/services-ci.yml`. The order above is fully serial.

## Cross-change check
pass (`cross_change_check: OK`). Binding cross-slice decisions are in `plans/training-platform/run/round2-decisions.md` (D1–D13).

## Open issues / NEEDS_HUMAN
Assumptions locked in. Confirm or change each before you sign:
- **D10 policy numbers:**
  - `recsys-als`: +1 % ndcg@10, coverage@10 guard 0.8× (ADR-0014)
  - `recsys-gbdt`: +0.5 % ndcg@10, recall@10 guard 0.95×
  - `recsys-two-tower`: +0.5 % recall@10, coverage@10 guard 0.8×, floor recall@10 ≥ 0.000001 (coverage floor removed by the human 2026-10-13: unreachable on a real catalogue)
  - GBDT: candidate below fixed-weight baseline × 1.01 is rejected structurally (human 2026-10-13)
  - Drift: the second run is promoted on a genuinely better dataset (human 2026-10-13)
  - `finetune-classifier`: +1 % macro_f1, macro_f1 floor 0.5
  - Thresholds are inclusive, with a 1e-9 tolerance.
- **Schedules:**
  - Recsys stays nightly at 03:00 with `activeDeadlineSeconds: 3600`.
  - Finetune runs Sunday 04:00 (`0 4 * * 0`) with `backoffLimit: 1`.
- **Two-tower holdout:**
  - The split is random and seeded, `TWO_TOWER_HOLDOUT_FRACTION=0.1`. There are no timestamps, so it cannot be temporal.
  - Fewer than 10 pairs means no candidate.
- **ALS has no floor.** Its first candidate, or one whose eval_protocol has changed, always passes. Add a floor if you want one.
- **Accepted residual risk (RAT-46).** If the registry goes down at `apply` after the Redis pointer swap, serving gets ahead of the registry. The job exits 1, and the guard refuses `rollback` until the next successful run.
- **Finetune egress:** TCP 443 to `0.0.0.0/0` minus cluster CIDRs, to download the HF dataset and the base model. The CIDRs and the GPU nodeSelector/toleration are taken from the gitops cluster config at build time.
- **No live GPU e2e for finetune.** Coverage is offline pytest on an MLflow file store plus static manifest tests.
- **Routed:** pii for recsys → MLflow. The scan covers params, metrics, system tags and artifact contents, with a planted-key negative control. No auth, payment or proto changes.
- `decide.py` was down in every round, so the verdicts above come from the LLM spec-quality-reviewer checklist.
- `scripts/validate_plan.py` refuses with "no task packets", because task packets are produced at build time. It is not a spec blocker.
