## 1. team-engagement

- [x] 1.1 ListReviews honours cursor/page_size, stable order, next_cursor/total (verify: handler + repository unit tests fail without the change)

## 2. team-frontend

- [x] 2.1 PDP requests the server page for `?rpage` and `?rating` (verify: gateway + ReviewList/page unit tests)

## 3. platform-e2e

- [x] 3.1 `rvp_` feature: 105 reviews seeded through the gateway, 11 pages, page 11 oldest, bounds, bad cursor; FEATURES entries

## Evidence (2026-10-10)

- team-engagement `make check` passes (handler and Postgres tie-order tests fail without the change). team-frontend `npm run check` passes (1219 tests).
- After rebuilding team-engagement and team-frontend, rvp gateway and UI scenarios plus the d1 PDP scenarios passed 38/38, twice.
