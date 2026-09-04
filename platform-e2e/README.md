# platform-e2e

Python end-to-end test **platform** for the marketplace polyrepo, built with
**pytest-bdd + Playwright** (sync API). It plays the role the `platform-e2e` row
in the root `AGENTS.md` describes — a standalone golden-path / E2E runner — as a
reusable framework any feature team plugs their scenarios into.

Design is ported from the `bds-qa-e2e-web` Cucumber+Playwright suite: Page Object
Model, a per-scenario World, page/service factories, tag-driven API seeding, and
env-based config — re-expressed in idiomatic Python.

## What it drives

- **UI**: `team-frontend` (Next.js, `http://localhost:3000`).
- **API (seeding + hybrid auth)**: `team-gateway` (Connect/JSON, `http://localhost:8080`).

## Quickstart

```bash
cd platform-e2e
make install                 # deps + Playwright Chromium
```

Bring up the app under test (from the repo root — see `platform-core` / root `AGENTS.md` §5):

```bash
docker compose -p platform-core up -d          # infra
# start the Go service containers (gateway, identity, listing, ...)
cd team-frontend && npm run dev                 # http://localhost:3000
platform-core/tools/seed-marketplace.sh         # seed accounts + catalog
```

Then run:

```bash
make collect     # dry-run: verify feature<->step wiring (no browser)
make smoke       # @smoke scenarios, headed, with HTML report
make buyer       # buyer journeys
make seller      # seller journeys
make auth        # UI login scenarios
make wap         # mobile emulation (DEVICE='iPhone 12')
pytest -m smoke -n 2   # parallel via pytest-xdist
```

Select an environment with `ENV` (default `local`): `ENV=staging pytest -m smoke`.

## How a feature adds an E2E test

1. Write a `.feature` under `tests/e2e/features/<area>/` with tags
   (`@smoke`, `@buyer`, seeding tags like `@needsSeller` / `@needsListing`).
2. Add steps in `tests/e2e/step_definitions/<area>_steps.py` — reuse
   `common_steps.py` first; call page objects, never assert inside page objects.
3. Bind them with a `test_<area>.py`: `scenarios("<area>/<file>.feature")` plus
   `from ...<area>_steps import *`.
4. Need a new page? Add `src/pages/<name>_page.py`, export it in
   `src/pages/__init__.py`, and register it in `src/core/page_factory.py`.
5. Heavy precondition? Add a payload row to `config/tags.py` (`TAG_PAYLOAD_MAP`)
   and tag the scenario — `seed_by_tags` seeds it via the gateway API before steps.

## Layout

```
config/     settings, browser/device options, TAG_PAYLOAD_MAP
src/core/   BasePage, BaseComponent, PageFactory, ServiceFactory
src/pages/  page objects (+ components/)
src/api/    gateway service clients (seeding + hybrid auth)
src/utils/  TestDataManager, faker(vi_VN), logger
tests/      conftest (fixtures/hooks/seeding), e2e/{features,step_definitions,flows,support}
scripts/    report + Datadog upload (stub)
```

See `AGENTS.md` for the conventions (locator priority, no fixed sleeps, reuse-first).
