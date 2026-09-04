# [W1-T2] Recently-viewed history backend (team-engagement)

## Role
SE

## Objective
Per-user recently-viewed history in team-engagement: `RecordView` also appends to a per-user `view_history` (dedup, most-recent-first), and a new `GetRecentlyViewed` returns the caller's recent listing ids, paginated. `make proto && make test` green.

## Write-set (EXCLUSIVE — nothing outside team-engagement/)
- team-engagement/migrations/0006_view_history.up.sql / .down.sql — new table (create)
- team-engagement/internal/repository/engagement_pg.go — extend RecordView to upsert history + add GetRecentlyViewed query (edit)
- team-engagement/internal/repository/engagement.go — repo interface method (edit)
- team-engagement/internal/service/engagement*.go — service method (edit; if RecordView/recently-viewed logic lives in a service file, add there)
- team-engagement/internal/handler/engagement.go — GetRecentlyViewed handler (edit)
- corresponding `*_test.go` (edit/create)
- team-engagement/internal/generated/** — via `make proto` (do not hand-edit)

## Read-only dependencies
- platform-core/packages/proto/platform/engagement/v1/engagement.proto (contract, Wave 0)
- Existing `ListFavorites` (repo+service+handler) + `RecordView` (repository/engagement_pg.go:104) = reference implementation.

## Contracts you implement
```proto
rpc GetRecentlyViewed(GetRecentlyViewedRequest) returns (GetRecentlyViewedResponse);
message GetRecentlyViewedRequest  { platform.common.v1.PageRequest page = 1; }
message GetRecentlyViewedResponse { repeated string listing_ids = 1; platform.common.v1.PageResponse page = 2; }
```
`view_history`: (user_id, listing_id, viewed_at) with a unique (user_id, listing_id) upsert-on-view (bump viewed_at); read = ORDER BY viewed_at DESC, cursor-paginated. Anonymous callers: RecordView keeps working (no history row), GetRecentlyViewed returns empty.

## Reference implementation
Mirror `ListFavorites` for GetRecentlyViewed (same handler/service/repo shape, same PageRequest→PageResponse). Mirror the existing `RecordView` PG write for the history upsert (add alongside the existing `listing_stats` counter, same method).

## Acceptance criteria
- [ ] RecordView still increments the aggregate counter AND upserts history (viewed_at bumped, no duplicate rows)
- [ ] GetRecentlyViewed returns caller's ids newest-first, excludes other users
- [ ] Anonymous principal → RecordView no-ops history, GetRecentlyViewed returns empty (no error)
- [ ] ≥3 tests: record→read roundtrip, dedup bump, cross-user isolation
- [ ] `make proto && make test` green

## Review (different agent)
SE rubric. New authed read RPC + boundary → auth-scope-reviewer + contract-boundary-reviewer; require CLEAN.

## Verify
cd team-engagement && make proto && make test

## Out of scope
- Do NOT put this in team-analytics.
- Do NOT touch team-gateway or team-frontend.
- Do NOT edit platform-core proto (Wave 0 did it). Do NOT alter reviews/qa/wishlist code.
