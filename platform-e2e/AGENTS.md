# AGENTS.md — platform-e2e

Working guide for this Python E2E platform. Read before adding or changing tests.
Architecture is ported from `bds-qa-e2e-web`; this file is the local contract.

## Architecture boundaries (do not cross)

- `tests/e2e/features/**.feature` — Gherkin only. No Python, no logic.
- `tests/e2e/step_definitions/` — Cucumber bindings + **assertions live here**.
  Shared/generic steps in `common_steps.py`; domain steps per area.
- `tests/e2e/flows/` — reusable API+UI orchestration (login, seeding). Extract here
  when logic is used by ≥2 step files.
- `tests/e2e/support/` — `world.py` (World), `state.py` (ScenarioState).
- `tests/conftest.py` — fixtures, bdd hooks, tag-scoped seeding.
- `src/core/` — `BasePage`, `BaseComponent`, `PageFactory`, `ServiceFactory`.
- `src/pages/` — locators + actions only. **No business assertions in page objects.**
- `src/api/` — gateway service clients (`*_service.py` extending `BaseService`).
- `config/`, `src/constants/`, `src/utils/`, `test-data/`.

## Hard rules

1. **Locator priority**: `get_by_role` → `get_by_label` → `get_by_placeholder` →
   `get_by_text` → `get_by_test_id` → CSS `locator` (last resort). No XPath.
   (The app currently ships **no `data-testid`** — prefer role/label/`#id`/VN text.)
2. **No fixed sleeps** — never `page.wait_for_timeout()`. Use `expect(...)`,
   `wait_for_url`, `wait_for_load_state`, locator auto-waiting.
3. **No assertions in page objects/components** — only in step definitions.
4. **Barrel imports**: import from `src.pages`, `src.constants`, `src.utils`,
   `src.api.services`. New page/component/service must be added to its `__init__.py`
   (and pages also to `PageFactory`).
5. **Reuse-first**: check `common_steps.py` before adding a step. Promote a step to
   `common_steps.py` when used by ≥2 features.
6. Page methods return values or `None`; steps take the `world` fixture, log via
   `world.logger`, and share data through `world.state`.

## Tags → markers

Gherkin tags become pytest markers (register new ones in `pyproject.toml`).
- `@smoke` critical path · `@buyer` / `@seller` / `@auth` persona
- `@wap` mobile emulation (`DEVICE='iPhone 12'`)
- `@needsSeller` / `@needsBuyer` seed an account · `@needsListing` seed a listing
  (payload in `config/tags.py::TAG_PAYLOAD_MAP`). Seeding runs via `seed_by_tags`
  before steps — never seed heavy data inside `@smoke`.

## Validate before a PR

```bash
make check       # ruff + black --check
make collect     # pytest --collect-only (feature<->step wiring)
make buyer / make seller / make auth   # affected suites (stack must be up)
```

## Hybrid auth

`tests/e2e/flows/auth_flow.py::login_via_api` logs in via the gateway and injects
the JWT as the frontend `session` cookie (fast path). `login_via_ui` drives the
real form (used only to test login itself). The `session` cookie value **is** the
raw gateway JWT — if the frontend ever wraps it, switch the default to UI login.
