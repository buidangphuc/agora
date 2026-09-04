---
name: spec-to-e2e
description: Turn an OpenSpec change into platform-e2e coverage. Reads the change's spec scenarios, updates the owning repo's FEATURES.yaml, scaffolds the .feature + steps + page objects in platform-e2e, runs them against the local stack, and flips the feature status to automated. Use after /opsx:propose (to plan e2e early) or during /opsx:apply (to build the e2e track), or whenever a change's user-facing scenarios need tests.
allowed-tools: Bash, Read, Edit, Write, Glob, Grep
license: MIT
compatibility: Requires the local marketplace stack running (frontend :3000, gateway :8080) and platform-e2e set up.
metadata:
  author: platform-e2e
  version: "1.0"
---

# spec-to-e2e — the e2e track of a spec-driven change

You are wiring an OpenSpec change into the **platform-e2e** test platform so the
change's `#### Scenario:` blocks become runnable e2e tests. One spec scenario ↔ one
`FEATURES.yaml` acceptance line ↔ one `.feature` scenario ↔ a `covered_by` pointer.

## Inputs
- A change id (else run `openspec list` / ask which change).
- Read its spec deltas: `openspec show <change-id>` (and the files under
  `openspec/changes/<change-id>/specs/<capability>/spec.md`).

## Ground rules (read these first)
- Conventions: `platform-e2e/AGENTS.md` (locator priority, no fixed sleeps, no
  assertions in page objects, barrel imports, reuse-first steps).
- Manifest contract + how auto consumes it: `platform-e2e/docs/FEATURE_MANIFEST.md`;
  schema `platform-e2e/schemas/features.schema.json`.
- Already-probed API payloads / selectors (reuse, don't re-derive):
  `platform-e2e/docs/AUTOMATION_BACKLOG.md`.
- Ownership: a feature lives in the repo that OWNS the capability (frontend is the UI
  shell only); `services:` lists every repo the journey touches.

## Steps
1. **Map** each spec scenario to a feature: pick the owning repo, choose/append a
   `feature.id` (`<domain>.<slug>`) in `<repo>/FEATURES.yaml`, fill
   persona/entry_route/services/preconditions(seed_tag)/api/ui.key_elements, and put
   the scenario's WHEN/THEN into `acceptance` (Gherkin-ready). Set `status: planned`,
   `covered_by: null` for now.
2. **Scaffold** in platform-e2e using the standard loop:
   - `.feature` under `tests/e2e/features/<area>/` (tags = markers; register any new
     marker in `pyproject.toml`).
   - Steps in a NEW `<area>_steps.py` (reuse `common_steps`:
     `the "<page>" page is open`, `I navigate to the "<page>" page`,
     `I am logged in as a buyer/seller via API`). Never edit another change's step file.
   - New page object → add to `src/pages/__init__.py` + `src/core/page_factory.py` +
     `src/constants/pages.py`. Assertions only in steps.
   - Binder `test_<area>.py`: `scenarios(...)` + `from ..._steps import *`.
   - Seeding: reuse tag hooks (`@needsSeller/@needsBuyer/@needsListing/@needsAddress/@needsOrder`).
3. **Run** against the live stack: `cd platform-e2e && . .venv/bin/activate &&
   HEADLESS=true python -m pytest tests/e2e/step_definitions/test_<area>.py`.
   Fix locators/timing (wait on real signals, never `wait_for_timeout`). If a
   scenario is blocked by a real backend gap, keep it `planned`/`manual` and add a
   `notes: BLOCKED: ...` line rather than papering over it.
4. **Flip** each green feature: `status: automated`,
   `covered_by: "<area>/<file>.feature::<Scenario name>"`.
5. **Gate**: `make -C platform-e2e features-check` (must exit 0) and
   `ruff check . && black --check .`.

## Output
Report per scenario: automated (with covered_by) / blocked (with reason). The change
is e2e-ready when every user-facing scenario is `automated` and green.
