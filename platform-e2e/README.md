# platform-e2e

Python end-to-end test **platform** for the marketplace polyrepo, built on **pytest-bdd +
Playwright** (sync API). It is a test runner, not a service: it owns no database, serves no
RPCs and publishes no events. It is not deployed. Locally it runs on the host, and the optional
docker runner is a one-shot container. Feature teams plug in scenarios; the platform also hosts
the repo-wide coverage gates (`FEATURES.yaml`, `spec-check`).

Design is ported from the `bds-qa-e2e-web` Cucumber+Playwright suite: Page Object Model, a
per-scenario World, page/service factories, tag-driven API seeding and env-based config.

## What it drives (contract)

- **UI**: `team-frontend` (Next.js, `BASE_URL`, default `http://localhost:3000`).
- **API (seeding, hybrid auth, probes)**: `team-gateway` Connect/JSON (`GATEWAY_URL`, default
  `http://localhost:8080`). Endpoint paths are in `src/constants/gateway_endpoints.py`; clients
  are in `src/api/`. The gateway enforces authorization; the tests only probe it (for example
  cross-user access checks).
- **Kafka** (read-only consumers, `confluent-kafka`): `analytics.events` (tracking envelopes
  produced by the gateway) and `order.events` (team-order outbox facts). Flows:
  `tests/e2e/flows/tracking_flow.py`, `order_events_flow.py`. Nothing is produced.
- **Flipt** (`FLIPT_URL`): toggles flags per scenario (`flipt_flow.py`), notably
  `checkout-enabled`.
- **Docker CLI** on the host: fault injection and the recsys job (see below).

Events produced: none. Data: none (test accounts and listings are created through the gateway;
fixtures are in `test-data/`).

## Test lanes

Markers are registered in `pyproject.toml` (about 35, `--strict-markers`: an unregistered tag
fails collection). Select with `pytest -m <marker>`.

| Lane | Command | Notes |
|---|---|---|
| Wiring check | `make collect` | `pytest --collect-only`, no browser, no stack needed |
| Smoke | `make smoke` | `@smoke`, headless, JUnit + self-contained HTML report in `test-results/` |
| Personas | `make buyer`, `make seller`, `make auth`, `make wap` | `wap` = smoke with `DEVICE='iPhone 12'` |
| Full, serial | `make test` | Everything, including `destructive`, one worker, with report |
| Parallel lane | `pytest -n 4 -m "not destructive"` | No Make target; pytest-xdist |
| Destructive lane | `pytest -m destructive` | No Make target; serial, run after the parallel lane |
| Docker runner | `make docker-build`, `make docker-test` | See "Docker runner" |

`destructive` scenarios stop a real stack service, flip a stack-wide switch
(`checkout-enabled`) or need an idle stack; they restore state in teardown and must never run in
parallel. They live in `auth/session_revocation`, `notification/delivery_hardening`,
`feature_flags/checkout_kill_switch`, `frontend/search_facets` and `ops/cockpit_metrics`.
Headed mode: `HEADLESS=false make smoke`.

## Configuration

`config/settings.py` is the single source. Load order (first value wins, shell included):
shell env, then `.env`, then `env/.env.<ENV>` (`ENV` defaults to `local`; files exist for
`local`, `docker`, `staging`). Because `.env` sets `HEADLESS=true` and is loaded first,
`HEADLESS=false` in `env/.env.local` has no effect; use the shell variable. There is no
`.env.example` and no drift gate.

| Variable | Default | Purpose |
|---|---|---|
| `ENV` | `local` | Selects `env/.env.<ENV>` |
| `BASE_URL` | `http://localhost:3000` | Frontend |
| `GATEWAY_URL` | `http://localhost:8080` | Gateway |
| `HEADLESS` | `true` | Browser mode |
| `DEBUG` | `false` | Debug logging |
| `DEVICE` | unset | Playwright device emulation, e.g. `iPhone 12` |
| `SEED_PASSWORD` | `pass123` | Password of seeded accounts (`seed-marketplace.sh` uses `pass123`) |
| `KAFKA_BROKERS` | `localhost:9092` | Broker for the consumers |
| `KAFKA_ANALYTICS_TOPIC` | `analytics.events` | Tracking assertions |
| `KAFKA_ORDER_TOPIC` | `order.events` | Order-fact assertions |
| `FLIPT_URL` | `http://localhost:8080` | Flag toggling |
| `SEARCH_CONTAINER` | `agora-team-search-svc` | Stopped by the search backend-failure scenario |
| `GATEWAY_CONTAINER` | `agora-team-gateway-svc` | Restarted by session revocation |
| `NOTIFICATION_CONTAINER` | `agora-team-notification-svc` | Restarted by notification hardening |
| `RECSYS_IMAGE` | `platform-recsys:local` | Image run by the recsys job scenarios |
| `STACK_NETWORK` | `platform-core_default` | Docker network for the job container |

The timeout fields on `Settings` (`action_timeout_ms`, `navigation_timeout_ms`,
`expect_timeout_ms`) are not read from the environment by `get_settings()`.
`scripts/upload_test_results.py` reads `DD_API_KEY` / `DATADOG_API_KEY` (stub).

## Run locally

Bring up the app under test as in the root `AGENTS.md` section 5, plus the root
`docker-compose.services.yaml` for the Go services and frontend (project `marketplace-services`,
attached to the external network `platform-core_default`):

```bash
(cd platform-core/infra && docker compose -p platform-core up -d)   # infra
docker compose -f docker-compose.services.yaml up -d --build         # services (repo root)
platform-core/tools/seed-marketplace.sh                              # seed accounts + catalog
```

Then:

```bash
cd platform-e2e
make install        # pip install -e ".[dev]" + Playwright Chromium
make collect
make smoke
```

The stack must be up for anything but `make collect`. Select an environment with `ENV`
(`ENV=staging pytest -m smoke`).

### Docker runner

`make docker-build` / `make docker-test` use `docker-compose.e2e.yaml`: container
`platform-e2e-runner`, `ENV=docker`, joined to the **external** network `platform-core_default`
(it must already exist, i.e. the stack is up). The image runs plain `pytest -q --tb=short`,
so it includes `destructive` scenarios. The root `docker-compose.services.yaml` also defines a
`platform-e2e` service (image `platform-e2e:local`) with the same `ENV=docker` settings.

### Fault-injection scenarios

`tests/e2e/flows/stack_flow.py` shells out to `docker stop` / `docker start` on the container
names in the table above (a genuine failure, no test hooks in production code) and waits for
recovery through the gateway. Run these only on an idle stack, serially, with the docker CLI
available on the host.

### Recsys job scenarios

`recommendations/pipeline_eval_registry.feature` (`@recsys @batch @registry`) runs the real
`platform-recsys` batch job as a black box via `tests/e2e/flows/recsys_job_flow.py`: `docker run`
of `RECSYS_IMAGE` on `STACK_NETWORK`, with `recsys_job_driver.py` mounted as the program. The
driver writes the Parquet from the scenario events, runs `python -m recsys` and snapshots the
stores. Prerequisites: the image must be built beforehand (the root compose service
`platform-recsys`, profile `jobs`, tags it `platform-recsys:local`), and Redis and Qdrant must
be running on the network (hosts `redis`, `qdrant`). Isolation is per xdist worker: Redis DB
`10+N` and Qdrant collections `e2e_recsys_gwN_*`; live serving data (Redis DB 0, default
collections) is not touched.

### Order / inventory scenarios (short reservation TTL)

The reservation-expiry scenarios need `team-domain` and `team-order` to expire holds in seconds,
not 15 minutes. `compose/order-inventory.override.yaml` sets `RESERVATION_TTL=20s` and
`RESERVATION_SWEEP_INTERVAL=2s` on both services. Layer it as an extra `-f` on the stack, from
the workspace root, whatever the compose project name is (it names services only, no container
names; add `-p <project>` if your stack runs under a non-default project, and any extra override
you already use as another `-f`):

```bash
docker compose -f docker-compose.yaml -f platform-e2e/compose/order-inventory.override.yaml up -d
# check: shows the values; plain `docker compose config` does not
docker compose -f docker-compose.yaml -f platform-e2e/compose/order-inventory.override.yaml config | grep RESERVATION
```

Only uncommitted holds expire that fast; placed orders are committed and immune to the sweep, so
the whole suite can run with the overlay. TTL scenarios wait TTL + 2 x interval + margin. Do not
use it for deployed environments (their defaults, 15m / 1m, apply).

### Payment and search scenarios (short hold window, short tombstone TTL)

Two more overlays shorten windows the same way, and the full suite runs with all of them:

- `compose/payment-ledger.override.yaml` sets `PAYOUT_HOLD_WINDOW=20s` on `team-payment`, so a fresh
  sale's proceeds leave the payout hold in seconds instead of `PAYOUT_HOLD_DAYS` (7). The hold-back
  scenarios and the bank-payout scenario wait the window out and fail fast, naming this file, without it.
- `compose/search-tombstones.override.yaml` sets `TOMBSTONE_TTL=30s` and a 2s purge interval on
  `team-search-indexer`, for the tombstone purge scenario (destructive lane).

```bash
docker compose -f docker-compose.yaml \
  -f platform-e2e/compose/order-inventory.override.yaml \
  -f platform-e2e/compose/payment-ledger.override.yaml \
  -f platform-e2e/compose/search-tombstones.override.yaml up -d
```

Deployed environments keep the defaults (7-day hold, 336h tombstone TTL).

## Build, test, lint

| Command | Does |
|---|---|
| `make check` | `ruff check .` + `black --check .`, the pre-PR gate |
| `make format` | `black .` |
| `make collect` | feature-to-step wiring |
| `make features-check` | `scripts/features.py --strict`, the coverage gate |
| `make clean` | removes `test-results`, caches |

Ruff and black use line length 100, Python 3.10+. There is no CI workflow in this repo; treat
`make check`, `make collect` and `make features-check` as the CI-equivalent gate.

## Spec and verification

- `platform-e2e` has no `FEATURES.yaml` of its own. Each owning repo ships
  `<repo>/FEATURES.yaml` (17 today); `scripts/features.py` globs `*/FEATURES.yaml`, validates
  against `schemas/features.schema.json`, checks that each `covered_by`
  (`<path under tests/e2e/features>::<Scenario name>`) resolves to a real scenario, and prints
  coverage. Convention and schema: `docs/FEATURE_MANIFEST.md`; backlog:
  `docs/AUTOMATION_BACKLOG.md`.
- `make features` reports; `make features-check` fails on any manifest or coverage error.
- `make spec-check CHANGE=<id>` runs `features-check` plus `scripts/spec_sync.py`: every
  `#### Scenario:` in `openspec/changes/<id>/specs/**/spec.md` must match an `automated`
  feature whose `covered_by` scenario exists (name match is normalized, substring either way).
  This is the archive gate in the root `AGENTS.md`.
- Changes go through OpenSpec (`openspec/changes/<id>`) per the root README's ASDLC. The
  `spec-to-e2e` skill scaffolds feature, steps and page objects and flips the status to
  `automated`.

## How a feature adds an E2E test

1. Write a `.feature` under `tests/e2e/features/<area>/` with tags (`@smoke`, a persona,
   seeding tags like `@needsSeller` / `@needsListing`). Name the Scenario after the spec scenario.
2. Add steps in `tests/e2e/step_definitions/<area>_steps.py`; reuse `common_steps.py` first.
   Page objects never assert.
3. Bind them in `tests/e2e/step_definitions/test_<area>.py`: `scenarios("<area>/<file>.feature")`
   (relative to `bdd_features_base_dir = tests/e2e/features`) plus
   `from .<area>_steps import *`.
4. New page: `src/pages/<name>_page.py`, export in `src/pages/__init__.py`, register in
   `src/core/page_factory.py`.
5. Heavy precondition: add a row to `config/tags.py` (`TAG_PAYLOAD_MAP`) and tag the scenario;
   `seed_by_tags` seeds it via the gateway before steps.
6. New tag: register it in `pyproject.toml` `markers`.

## Layout

```
config/     settings, browser/device options, TAG_PAYLOAD_MAP
src/core/   BasePage, BaseComponent, PageFactory, ServiceFactory
src/pages/  page objects (+ components/)
src/api/    gateway service clients (seeding + hybrid auth)
src/utils/  TestDataManager, faker(vi_VN), logger
tests/      conftest (fixtures/hooks/seeding), e2e/{features,step_definitions,flows,support}
scripts/    features.py + spec_sync.py (coverage gates); generate_report.py, upload_test_results.py
schemas/    features.schema.json
env/        .env.local / .env.docker / .env.staging
```

## Gotchas

- **Hydration**: server-rendered controls are visible and enabled before React attaches
  handlers, and an early click is silently dropped. Use
  `BasePage.wait_until_interactive(locator)` (`src/core/base_page.py`): it re-resolves the
  locator and polls until the element carries React's `__reactProps$` key. Used by the login,
  register, addresses and seller listing pages and several step files. Never replace it with a
  sleep.
- `--strict-markers`: unregistered tags error out.
- The `session` cookie is the raw gateway JWT (`login_via_api`); if the frontend wraps it,
  switch the default to UI login.
- `docker run` / `docker stop` scenarios need the docker CLI and the stack on
  `platform-core_default`.
- Duplicate feature directories exist: `feature-flags` / `feature_flags` and `ops` /
  `ops-cockpit` (both hold a `cockpit_metrics.feature`).

## Known gaps

- `AGENTS.md` rule 2 forbids `wait_for_timeout`, but it is used in
  `BasePage.wait_until_interactive` (100 ms poll) and in fixed waits in `pdp_streaming_steps.py`,
  `ui_components_steps.py`, `product_detail_steps.py`, `pdp_shop_steps.py` and
  `discovery_steps.py`.
- No Make targets for the parallel or destructive lanes; `make test` and `docker-test` run
  everything serially, destructive included.
- No CI workflow in this repo, and no `.env.example` drift gate.
- `env/.env.local` `HEADLESS=false` is inert (see Configuration); timeouts are not env-configurable.
- `scripts/upload_test_results.py` is a stub.

## Links

Rules: root `AGENTS.md` and `platform-e2e/AGENTS.md` (locator priority, reuse-first, no
assertions in page objects). ADRs: `platform-core/docs/ADR/`, notably 0009
(payment-order event integration).
