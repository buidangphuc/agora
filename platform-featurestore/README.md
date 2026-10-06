# platform-featurestore

**Status: unused Python library prototype. Not deployed, not imported by anything.** It is not a
service: no server, no port, no compose entry, no proto, no events, no database. The Dockerfile
only runs the unit tests. Nothing else in the agora polyrepo imports `featurestore`.
`team-ai` has its own `FeatureStorePort` / `InMemoryFeatureStore`
(`app/modules/business/recommend/ranking.py`), and its factory wires `InMemoryFeatureStore()`.

The system of record for features is the feature store in **team-analytics** (C2, ADR-0015,
status Proposed; DuckDB-backed). This package was scaffolded by the OpenSpec change
`add-platform-featurestore` (P3-T2) and predates that decision. Treat it as a reference for the
`UserFeatures` / `ItemFeatures` shape and the online/offline parity check, not as shared
infrastructure. A decision is needed before extending it: wire it in, move it, or delete it.

Bounded context: ML/recsys feature schemas and in-process stores. Owns no data.

## Contract

None served or consumed. Public Python API (`featurestore/__init__.py`):

| Symbol | Behaviour |
| --- | --- |
| `UserFeatures`, `ItemFeatures` | Dataclasses (`definitions.py`) with `to_dict/from_dict/to_json/from_json`. Fields are listed in that file. |
| `OnlineFeatureStore(redis_client=None, prefix="fs:")` | Get/set per user and item, plus `get_item_features_batch`. Keys `fs:u:<user_id>` and `fs:i:<listing_id>`, JSON values, `setex` with `ttl_seconds=86400` by default. Batch read uses `mget`. |
| `OfflineFeatureStore()` | In-memory dicts keyed by id (`offline.py`). Last write wins. `build_training_dataset(interactions)` joins `user_id`/`listing_id`/`label` rows with features, prefixing columns `u_` / `i_`. |
| `validate_parity(online, offline, user_ids, item_ids, tolerance=1e-4)` | Returns `ParityReport(is_consistent, total_checked, mismatches)`. Numbers compared with `math.isclose(rel_tol=abs_tol=tolerance)`. |

## Events

None.

## Data

None. No database, tables or migrations. Redis is optional and only reached through an injected
client; the package never opens a connection. Without a client, `OnlineFeatureStore` keeps a
per-process dict (not shared, TTL ignored).

## Configuration

None. The code reads no environment variables; there is no `.env.example` and no drift gate.
Redis client, key prefix and TTL are constructor / method arguments.

## Run locally

Library only; there is no service and it is not in the root `docker-compose.services.yaml`.
Standalone:

```bash
cd platform-featurestore
pip install -r requirements.txt   # pydantic, redis, pytest, ruff (pyproject: python >=3.10)
docker build -t platform-featurestore . && docker run --rm platform-featurestore   # runs pytest on python 3.12
```

## Build, test and lint

| Command | Does |
| --- | --- |
| `make test` | `pytest -v tests/` (`test_featurestore.py`, `test_parity.py`) |
| `make lint` | `ruff check .` (line length 110, target py310) |
| `make format` | `ruff format .` (missing from `.PHONY` in the Makefile) |

There is no CI workflow for this repo. Run `make lint test` before a PR; `pytest` and `ruff` must
be installed first (`pip install -r requirements.txt`).

## Spec and verification

- Change: `openspec/changes/add-platform-featurestore` (tasks all ticked). Related:
  `wire-serving-gbdt-featurestore`, which wires the `team-ai` port, not this package. Further
  changes go through `openspec/changes/<id>` per the root README's ASDLC.
- `FEATURES.yaml` has one entry, `featurestore.online-features`, status `not-testable`: there is
  no service or route to drive from platform-e2e. Gates: `make -C platform-e2e features-check`
  and `make -C platform-e2e spec-check CHANGE=<id>`.

## Gotchas

- The "offline store" is an in-memory dict, not Parquet/DuckDB and not point-in-time. There are
  no timestamps and no as-of join; `build_training_dataset` is a plain key join (missing features
  yield rows without those columns).
- The proposal promised Parquet/DuckDB and sub-5ms lookups; neither is implemented or measured.
- `validate_parity` skips ids absent from both stores but still counts them in `total_checked`.
- `OnlineFeatureStore` falls back silently to memory when `redis_client` is falsy.

## Known gaps

- Nothing imports this package; the `use_featurestore` flag in `team-ai` placements does not use it.
- Overlaps with the team-analytics feature store (ADR-0015). No decision is recorded on retiring it.
- `FEATURES.yaml` is inconsistent: the summary says sub-5ms, the acceptance says 10ms; `entry_route: /`
  and persona `guest` do not fit a library.
- Not listed in the root `AGENTS.md` repo table or in `docker-compose.services.yaml`.
- `pydantic` is declared as a dependency but no module imports it.
- No CI workflow.

## Links

- Root `AGENTS.md` (rules, repo table).
- ADR-0015, feature store in team-analytics: `platform-core/docs/ADR/0015-feature-store-in-team-analytics.md`
  (present in the full_team_repo checkout, not in agora).
