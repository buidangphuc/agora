# gates/ — versioned question sets for the decision engine

Each file is a closed checklist that Kev answers once per **state unit** (one scenario,
one task or one change). `scripts/asdlc/decide.py` turns the answers into
`PASS / FAIL / ESCALATE_LLM / BLOCK` (rules in [SCHEMA.md](SCHEMA.md)). Every question
cites the standard it comes from in `source:`; nothing is invented. Only judgement
questions live here. Anything a machine can check is a script (list below).

## Gates

| Gate file | State unit (what the orchestrator puts in the state) | Standards (`source:`) | Questions (veto) |
|---|---|---|---|
| `spec.v1.yaml` | one requirement block: the `### Requirement:` statement, all of its `#### Scenario:` blocks, and the proposal's *Why* | ISO/IEC/IEEE 29148:2018 §5.2.5, EARS (Mavin et al. 2009), OWASP ASVS 4.0.3 V4, Decree 13/2023/ND-CP | 8 (2) |
| `task.v1.yaml` | one task: its line(s) and acceptance from `tasks.md`, the test diff written for it, its implementation diff | ISO/IEC/IEEE 29119-4:2021 (equivalence partitioning, boundary value analysis, error guessing), Meszaros *xUnit Test Patterns*, tsDetect, Google Eng Practices | 8 (2) |
| `feature.v1.yaml` | one scenario: the scenario text, its `.feature` text, the step code it runs, the implementation hunk(s) the steps reach | as task, plus OWASP ASVS 4.0.3 V4.1.1 / V4.2.1 for permission scenarios | 9 (4) |
| `release.v1.yaml` | one change: release note, migration files, rollout / GitOps manifest diff, alert-rule and runbook diff | Google SRE Book, ch. *The Evolving SRE Engagement Model* (Production Readiness Review), *Monitoring Distributed Systems*, *Release Engineering* | 7 (2) |

`veto` is used only where a fail means the test or spec cannot be trusted (e.g. assertions
only check a mock, a spec outcome no test can verify). A unit must stay under
`max_state_tokens` (8000) or decide.py returns `ESCALATE_LLM` without asking Kev.

Thresholds start at `threshold: 0.85` (noul) and `min_confidence: 0.6` (choice/score).
They are placeholders: W3-6 recalibrates them from the precision/recall curve on the
seeded-defect eval set (docs/asdlc/ASDLC-v2-PLAN.md §14).

## Versioning

- File name is `<gate>.v<N>.yaml` and must equal `gate` + `version` inside it.
- Any change to a question's wording, options, severity or threshold, or adding/removing a
  question, is a **new file** with `version: N+1`. The old file stays so logged runs can be
  replayed on the same state.
- `pins:` records the standard editions the `source:` IDs refer to (ASVS is pinned to 4.0.3;
  5.0 renumbers chapters). Bumping a pin is a new version.
- Validate before committing: `platform-e2e/.venv/bin/python scripts/asdlc/check_gates.py`
  (schema, enums, unique ids, thresholds in (0.5, 1), `pass_values` ⊆ `criteria`,
  `pass_min` ∈ `levels`, non-empty `source`, file name; exits 1 on any error).

## Handled by scripts (never Kev questions)

- Routing to `NEEDS_HUMAN`: change touches auth, payment, PII or a prod release — path globs.
- Edits outside the task's write-set — `scripts/validate_plan.py --audit`.
- Secrets in the diff — secret scanner.
- Proto / contract breaking change — `buf breaking`.
- A scenario or task has no test file / no e2e coverage — `FEATURES.yaml` + spec-check.
- Spec format: every requirement has a scenario, every scenario has WHEN/THEN, SHALL/MUST
  keyword, vague-word list, proposal has NFR / Data classification / Threat model sections
  — `scripts/asdlc/spec_lint.py` (Kev judges whether their content is adequate).
- E2E / integration test mocks one of the system's own services —
  `scripts/asdlc/no_self_mock.py` (Kev's `assertion_kind` covers the judgement part).
- Test passes before the change / fails after (red-green) — CI run, not a question.
- State unit above `max_state_tokens` — decide.py, before calling Kev.
