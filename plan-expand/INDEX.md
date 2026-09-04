# Plan Index — Expansion batch (6 features)

| Wave | Task | Slug | Owner repo | Verify | Status |
|---|---|---|---|---|---|
| 0 | W0 | proto-contracts | platform-core (6 domain protos); regen FE + service stubs | buf lint clean | **done** |
| 1 | F1 | account-safety | team-identity/** | make proto && make test | **done** (SessionService, green) |
| 1 | F2 | saved-searches | team-search/** | make proto && make test | **done** (5 tests green) |
| 1 | F3 | reorder | team-order/** | go test ./... | **done** (4 tests green) |
| 1 | F4 | loyalty-checkin | team-engagement/** | make proto && make test | **done** (5 tests green) |
| 1 | F5 | bundles | team-domain/** | make proto && make test | **done** (green) |
| 1 | F6 | sponsored | team-promotion/** | make proto && make test | **done** (SponsoredService, green) |
| 2 | I1 | gateway-forwarders | team-gateway/** | make check | **done** (6 features, 2 new svc handlers) |
| 2 | I2 | thin-frontend | team-frontend/** | tsc + vitest | **done** (6 UIs, tsc clean, 158 tests) |
| 3 | W3 | local-build | docker compose build/up | stack healthy | **done** (22 svcs up, FE 200 @ :3000) |

**Local run fixes:** (1) team-ai host port 8000→8001 (8000 held by unrelated `pynew-bench`); (2) team-analytics runtime image `distroless/base`→`distroless/cc-debian12` (DuckDB needs libstdc++) + `DUCKDB_PATH=/tmp/analytics.duckdb` (nonroot can't write the root-owned `/data` volume). All 18 images built; frontend renders real content; gateway clean.

All 6 features implemented + integrated (gateway + thin UI). Wave 3 = `docker compose up --build` for local browser testing at localhost:3000.
