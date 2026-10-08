## Context

See `proposal.md` - Why. Current state, verified in code (`team-search`, branch `fix/repo-doctor-search`):

- **Index (`internal/index/opensearch.go`).** `Search` builds `filter` clauses from `filters` (every key becomes a
  `term` on the same-named field), `category_id`, `price`, `min_rating`; nothing adds a status clause. `Suggest` is a
  bare `bool_prefix` `multi_match`. `SORT_BY_NEWEST` is `sort _id desc`. `Upsert` is a full replace under OpenSearch
  **external versioning** (version = envelope `occurred_at` in ns; 409 = stale = no-op). `PartialUpdate` is a painless
  version-guard script with no `upsert` clause (404 = no-op). `Delete` is a plain `DELETE`, 404 ignored. `EnsureIndex`
  only creates a missing index. `ListingDoc.Rating` is a non-pointer `float64` with no `omitempty`, so every upsert
  writes `rating: 0`. `facetAggs` always requests four rating filters and `parseFacets` always emits four buckets.
- **Consumer (`internal/consumer/listing.go`).** The three delete paths call `idx.Delete(ctx, id)` with no version.
  `toDoc` has no timestamp. `envVersion` returns 0 when `occurred_at` is nil (guard off). `ListingChanged` has
  `change_type` CREATED/UPDATED/DELETED but `Listing` carries **no timestamp field**; the only time is the envelope's
  `occurred_at`.
- **Handler.** `SearchListings` calls `RequireScopes("search:read")` then passes `req.GetFilters()` straight to the
  index. `callerID` checks scope, then only `p.GetId() != ""`. `interceptor.principalFromMetadata` parses
  `x-principal-type` into `PrincipalType` (`user|service|anonymous`), so the type is available to handlers and unused.
  `RunSavedSearch` ends in the same `h.idx.Search` call, with `saved.FiltersJSON` parsed into `filters`.
- **Gateway (`team-gateway/internal/edge`).** `authInterceptor` never rejects; with no or an invalid bearer token,
  `resolve` yields id `anonymous`, type `anonymous`, scopes `PUBLIC_SCOPES` (default `listing.read,search:read`).
  `SearchForwarder` forwards all six RPCs with `callRead` or `callWrite` and does **no** scope or identity gate.
  Identity (`team-identity/internal/authz/scopes.go`) grants `search:write` to buyer, seller and admin. So today the
  only thing stopping anonymous `SaveSearch` is that `PUBLIC_SCOPES` happens to omit `search:write`.
- **Saved-search storage.** `repository.PostgresSavedSearchRepository` (database/sql, every query scoped by `user_id`)
  and `migrations/0001_saved_searches` exist. `cmd/server/main.go` wires the in-memory repository and says so in a
  comment. `internal/config` has no `Database` group, `go.mod` has no Postgres driver, and `bootstrap.OpenResources`
  opens no pool. `docker-compose.services.yaml` already gives `team-search` and the indexer `DATABASE_ENABLED` and
  `DATABASE_URL` (ignored), and `search_db` / `search_svc` are created by
  `platform-core/infra/postgres-init/10-create-service-databases.sh`. Every other DB-backed service applies migrations
  through a one-shot `<svc>-migrate` golang-migrate container that the app `depends_on`; there is no `team-search-migrate`.
- **Boot-guard precedent (`team-order`).** `Settings.RequireDurableStorage(dbAvailable)` with
  `durableStorageEnvs = staging|stage|prod|production`; `cmd/server/storage.go` calls it before any port is opened and
  logs a warning for the in-memory case. `Database.Enabled` defaults `true`; `Validate` requires the URL when enabled.
- **Ratings.** `platform.engagement.v1` (owned by `team-engagement`) has `CreateReview`, `ListReviews`,
  `GetListingRatingSummary`, `GetShopRatingSummary` as RPCs. No `team-engagement` code publishes a Kafka event and no
  proto in `platform-core` defines a rating or review **event**. The listing events carry no rating.
- **Frontend.** Always sends `filters.status = "published"` (`src/lib/gateway/search.ts`); sends `minRating` only when
  the URL has `rating`; renders the ratings facet whenever `facets.ratings.length > 0` (so today it renders four
  zero-count buckets).
- **E2E.** `frontend/saved_searches.feature` has two scenarios for a logged-in buyer. It saves an empty filter set and
  a keyword, so it is unaffected by the stricter rules, and it passes today only because the service is single-process
  and never restarted.

Constraints: Rule 3 (team-search owns its index and its saved-search DB; ratings would have to come from an event or a
call to `team-engagement`, never a join), Rule 4 (no proto change), Rule 2 (the gateway gets no business logic), AD1
(consumer offset discipline) and AD2 (external version guard) unchanged.

## Goals / Non-Goals

**Goals:**

- Published-only is the default for every search path, enforced once, in one place, with a narrow explicit owner view.
- "Newest" means creation time and is stable under edits, redelivery, reordering and replay.
- Saved searches are durable, private to an authenticated user, and cannot silently run in memory in staging or
  production.
- A deleted listing is immune to older events for as long as a stale event can plausibly arrive, and the marker does
  not leak listing content or grow unbounded.
- The rating controls stop misleading buyers, at the smallest cost, without closing the door on a real source.

**Non-Goals (design-level):** no new index, alias or reindex (that is the hybrid change); no change to `version`
semantics of existing events; no per-user saved-search sharing; no pagination change (still `from/size`); no
allow-list redesign of arbitrary `filters` keys beyond `status` (noted under Risks); no admin override for drafts.

## Decisions

### D1 - Default `status=published` lives in the shared filter builder; authorization lives in the handler

`Index.Search` and `Suggest` always add a `term status=published` clause unless the filter set already contains a
`status` key, in which case that value is used. The handler is the authorization layer: before calling the index it
runs a small policy function `effectiveFilters(principal, filters)` that (a) accepts `status` only as `published`
or `draft`, (b) allows `draft` only for a `user` principal whose `filters["seller_id"]` equals its id, and (c) returns a
copy of the map (never mutating the request). Anything else returns the errors in the spec. `Suggest` has no filter map,
so the index clause is unconditional `status=published` (no owner variant: suggestions are a public surface).

Why split it this way: the **index** guarantees that no caller, present or future (saved-search run, hybrid's semantic
or structured strategies, which reuse the same builder per stock task 2.4 and hybrid's filtered kNN clause, stock D5), can forget the
default and leak drafts. The **handler** is the only place that knows who the caller is. This keeps `Index.Search`'s
signature unchanged, which matters because `search-stock-events` and `add-hybrid-retrieval-platform` both edit that
method and its callers; adding a principal parameter would collide with both.
*Alternative:* pass an explicit `Visibility` struct into `Index.Search`. Cleaner types, but it changes the interface
that two in-flight changes also change, for no extra safety, since the clause is already unconditional at the index.
*Alternative:* `must_not status=draft` instead of `filter status=published`. Rejected: it would admit `rejected`,
`unspecified` and the new `deleted` tombstone. A positive allow is the safe default.
*Alternative:* make `status` a typed proto field. A proto change for something the map already carries; additive but
not needed.

### D2 - The owner view: explicit `seller_id` + `status=draft`, for `user` principals only

A seller (any authenticated `user` principal; there is no seller-only scope today, identity gives `listing.write` to
sellers) sees their drafts with `filters {"seller_id": "<self>", "status": "draft"}`. Omitting `status` returns
published only, even with `seller_id`, so the same request never changes meaning based on who sends it. This is additive:
no existing well-formed request changes outcome except the buggy ones that relied on drafts leaking.
Who asks for drafts in practice: the seller center uses `team-domain.ListMyListings` (requires `listing.write`), not
search, so this view exists for completeness and for the e2e scenario, and is kept deliberately tiny.
*Alternative:* auto-inject `seller_id` from the principal with a new flag. Needs a proto field. Rejected.
*Alternative:* no draft view at all (published-only, period). Simplest and safest, but the requirement text asked for an
explicit way; one line of policy covers it and it is easy to delete. It is called out as Open Question 1.

### D3 - `created_at` as a first-class field, set by a commutative "earliest wins" update

The doc gets `created_at` (`date`, `epoch_millis`; millisecond resolution is enough for ordering, and `id` breaks ties).
Sort becomes `[{created_at: {order: desc, missing: _last, unmapped_type: date}}, {id: {order: desc}}]`; the `id`
keyword field already exists in the mapping, so no `_id` sort remains.

Source of the value: the `occurred_at` of a `ListingChanged` with `change_type = CREATED`. There is no timestamp on
`Listing` (verified in `platform/listing/v1/listing.proto`), and `occurred_at` is the same clock the indexer already
trusts for versions. Writing it, in two steps inside the `ListingChanged` branch:

1. `Upsert` includes `created_at` = the existing doc's `created_at` if one exists (carried through the same GET that
   stock D3 already performs before the external-versioned write), else the `CREATED` event's time, else unset. A
   full upsert is a replace, so without the carry-forward every seller edit would erase it.
2. For a `CREATED` event only, `SetCreatedAt(id, tsMillis)` then runs a scripted update: set when absent or when the new
   value is **earlier** than the stored one; `404` and a tombstone are no-ops. It is independent of the external version
   on purpose: replaying `listing.events` into a *live* index has each old `CREATED` upsert rejected as stale (409), yet
   the listing still needs its creation time; `min` is commutative and idempotent so ordering and redelivery cannot
   corrupt it.
Why not seed `created_at` from an `UPDATED` event: a legacy document edited after this ships would get "created
at edit time" and jump to the top of "newest". Better to leave it missing (sorts last) until a replay supplies the real
`CREATED` event (Migration Plan). This is the only place where a correct answer needs the topic's history.
*Alternative:* sort by `version` (occurred_at of the last event). That is "recently updated", and editing would reorder.
*Alternative:* a proto `created_at` on `Listing`, populated by team-domain. Correct and simplest to consume, but a
cross-team contract and domain change, plus a backfill there; out of proportion for an ordering fix. Listed as the
upgrade path.
*Alternative:* use the `id` if ids were UUIDv7. They are not guaranteed to be; team-domain owns id format.

Mapping: `created_at` is added by the same idempotent put-mapping call `EnsureIndex` issues for stock (stock D4) and to
the create-mapping for fresh clusters; a conflicting pre-existing `created_at` mapping fails startup loudly. It is
additive, so `add-hybrid-retrieval-platform`'s v2 mapping must include it (D9 coordination).

### D4 - Saved-search storage: config, pool, init-container migration, guard

- Config gains a `Database` group `{Enabled DATABASE_ENABLED default true, URL DATABASE_URL default ""}`, mirroring
  `team-order`. `Validate` does **not** require the URL (the indexer shares `LoadSettings` and needs no DB); the server
  checks it in `RequireDurableStorage` and when opening the pool.
- `internal/bootstrap` gains an optional pool opener. The repository uses `database/sql`, so the driver is
  `github.com/jackc/pgx/v5/stdlib` at `v5.6.0` (the version `team-order` and `team-domain` already pin and that builds
  on Go 1.22). This is the one new dependency; it is required by the feature, not incidental.
- `cmd/server` picks the repository: pool available -> Postgres; otherwise in-memory only when
  `!RequiresDurableStorage()`. `RequireDurableStorage` is a copy of `team-order`'s (same env list, same error text
  shape, same "log a warning for the in-memory case" helper) so the two services cannot drift on what "strict" means.
  The guard runs before `net.Listen`.
- **Migrations are applied the way every other service does it**: a `team-search-migrate` golang-migrate container in
  `docker-compose.services.yaml` (`-path=/migrations -database=postgres://search_svc:search_pass@postgres:5432/search_db`
  `up`) that `team-search` `depends_on` with `service_completed_successfully`. No in-process migration runner: the repo
  convention is init containers, `migrate up` is idempotent, and adding a second mechanism in one service is exactly
  the kind of drift the repo-doctor exists to prevent. The indexer does not depend on it.
- Tests: repository tests run against the in-memory store always, and against a real Postgres when
  `SEARCH_TEST_DATABASE_URL` is set (skip otherwise), the same pattern as `OPENSEARCH_TEST_URL` in
  `internal/index/integration_test.go`. A `make test-integration` target starts throwaway `postgres:16` and
  `opensearch` containers, applies `migrations/0001`, exports the URLs, runs the tagged tests and removes the
  containers.
*Alternative:* embed migrations and run them at server startup with golang-migrate as a library. Self-contained, but a
new pattern and dependency; two replicas race on the lock. Rejected for now.
*Alternative:* keep the in-memory repository everywhere and only document it. That is the bug.

### D5 - Saved-search privacy: the principal check is a precondition, not a scope side effect

`callerID` becomes: (1) principal present, else `UNAUTHENTICATED`; (2) type is `user`, else `UNAUTHENTICATED` when
`anonymous`/`unspecified` and `PERMISSION_DENIED` when `service`; (3) id non-empty and not the reserved value
`anonymous`, else `UNAUTHENTICATED`; (4) only then `RequireScopes`. Checking identity before scope makes the answer
for anonymous independent of `PUBLIC_SCOPES` (today, `SaveSearch` from anonymous yields `PERMISSION_DENIED` because
`search:write` is missing, but `ListSavedSearches` and `RunSavedSearch` succeed). The check is on the principal the
gateway forwards as trusted metadata (ADR-0003); team-search's port is not published, so the metadata is not
spoofable from outside the network.

Owner scoping is already correct in both repositories (`WHERE id = $1 AND user_id = $2`, and in-memory checks
`s.UserID != userID`); `Get` and `Delete` return `ErrSavedSearchNotFound` for a foreign id, which the handler maps to
`NOT_FOUND`. There is no read-by-id path other than `RunSavedSearch` (checked: `Get` is called only from it), and no
shared or admin path. The change therefore adds **tests**, not repository changes: anonymous and service callers denied
on all four RPCs, cross-user list/run/delete, `page.total` counting only the owner's rows, and the same tests run
against Postgres.

Save-time and run-time validation share D1's `effectiveFilters`, so a saved `status=draft` filter cannot be used to get
drafts later, and a non-owner cannot store one. The existing test that saves `{"status":"active"}` uses a
non-existent status and must be updated to a valid one.
*Alternative:* enforce at the gateway only. Rule 2 says the gateway authenticates once and forwards; authorization of
what a principal may do with its own data is the service's job, and a service that trusts the edge for this is what we
have now. The gateway follow-up (an early anonymous reject) is defence in depth, not the fix.
*Alternative:* treat `anonymous` as a session id and give each visitor a private bucket. Rejected: there is no stable
anonymous identity at the service (the id is a constant), and anonymous saved searches have no retrieval path.

### D6 - Tombstone as a version-guarded soft delete, with the status filter doing the hiding

`Delete(ctx, id, version)` indexes `{id, status: "deleted", version, deleted_at}` with `version_type=external`,
`version = occurred_at` nanos. This reuses the exact AD2 mechanism that orders every other write:

- a stale upsert (`version <= stored`) returns 409 and is a no-op, **with no time limit**, unlike OpenSearch's own
  delete-tombstone that expires after `index.gc_deletes` (60s default);
- a stale *delete* (older than a newer upsert) is itself a 409 no-op, which the current unconditional `DELETE` gets
  wrong (it removes a listing a later event updated);
- a delete for a listing that has no document creates the tombstone, so a create arriving later with an older version is
  rejected (delete-before-create reordering);
- the body holds no title, description or seller, so a deleted listing's content leaves the index (what a hard delete
  did) while its id and version stay.

Two things make the tombstone invisible and inert:

1. **Invisible**: every query already requires `status=published` (D1), and the handler rejects `status=deleted` from
   callers, so the tombstone is never matched by search, facets or suggest. There is no separate "exclude deleted" clause
   to forget.
2. **Inert**: the `PartialUpdate` script and stock's `UpdateStock` script get a first line `if (ctx._source.status ==
   'deleted') { ctx.op = 'noop'; }`. Without it a partial update with a newer version would overwrite `status` and
   revive a husk document. `SetCreatedAt` (D3) has the same guard. Only a full `ListingChanged` upsert with a version
   newer than the delete replaces it; the domain never emits an update after a delete, so a newer-than-delete upsert is
   treated as real state (external versioning semantics), and clock skew between replicas is the same accepted risk the
   indexer already carries.

`ListingStatusChanged(REJECTED)` keeps calling the delete path (today it calls `idx.Delete`), so it now tombstones too;
the behavior difference from before is only that a stale upsert can no longer bring it back. A seller fixing a rejected
listing produces a newer `ListingChanged`, which replaces the tombstone.

`occurred_at` absent (version 0): `envVersion` returns 0 and disables the guard for ordinary writes. A delete without a
timestamp is stamped with the indexer's wall clock so the tombstone still guards; this is a malformed event in practice
(every producer sets `occurred_at`) and the choice is documented in a code comment and a test.

*Alternative:* keep hard `DELETE` and raise `index.gc_deletes` (a per-index setting, e.g. `14d`). A one-line config with
no schema change, but it is cluster-side state that a new index, an alias flip (hybrid) or a replay into a scratch index
silently loses, and it does not fix stale deletes or delete-before-create.
*Alternative:* a separate `listing_deletes` index of ids. Two writes and a read on every upsert to check it; the single
versioned document gives the same guarantee atomically in OpenSearch's version check.
*Alternative:* a painless `delete` inside the update script. Does not cover a missing document or keep a version.

### D7 - Tombstone retention: indexer purge job, TTL 14 days

A stale event can be applied late only while it is still deliverable: redelivery after a crash, a consumer lagging, or
a DLQ re-drive. Replay from offset 0 is in order, so it converges without tombstones (spec scenario). The retention is
therefore "longest plausible delivery delay", set to **14 days** by default (`TOMBSTONE_TTL`, a Go duration), which is
comfortably longer than the local broker's retention and the DLQ re-drive habits, at a cost of a few tiny documents per
deleted listing. The `cmd/indexer` process runs a loop every `TOMBSTONE_PURGE_INTERVAL` (default `1h`) issuing
`_delete_by_query` with `bool filter [term status=deleted, range version < (now - TTL) in ns]`, conflicts `proceed`.
`version` is the delete event's nanosecond time, so the TTL is measured from the delete, not from when the tombstone was
written. The job lives in the indexer because it is the writer; the query server never writes the index. Its count is
logged and exposed as a metric; a failure is logged and retried next tick (never fatal).
*Alternative:* never purge. Simplest, and for a marketplace's delete volume tolerable for a long time, but unbounded
growth is a trap that nobody revisits. *Alternative:* OpenSearch ILM or a TTL field: not available for this use in the
pinned OpenSearch setup without extra plugins and policy objects.

### D8 - Rating: make the dead controls honest (recommended), keep the mapping

Three options were weighed; the recommendation is the smallest one that removes the lie without a cross-team decision.

| Option | What it takes | Verdict |
|---|---|---|
| A. Populate from events | A rating event is needed: `team-engagement` publishes none and no proto defines one, so this means a new event contract in `platform-core`, a `team-engagement` outbox/producer, a new consumer case, and a backfill. Or `team-search` calls `GetListingRatingSummary` per listing from the indexer (Rule 3 allows a gRPC call) and re-polls on every review. | Right long-term, a real feature, its own change. |
| B. Hide until a source exists | Return an empty `ratings` facet, reject `min_rating > 0` with `INVALID_ARGUMENT`, stop writing `rating: 0`. Code: one handler check, one facet branch, one omitempty. Mapping and proto unchanged. | **Recommended.** |
| C. Reject the filter only | Clear error for `min_rating`, but the facet still shows four zero buckets. | Half a fix; the UI keeps showing a dead control. |

B also fixes the latent clobber: `ListingDoc.Rating` becomes `*float64` with `omitempty`, so a full upsert no longer
writes `rating: 0` over a real value; when option A lands it must carry the rating forward exactly like stock's D3
(noted in the code comment). The mapping field stays (`float`), so turning A on later is a consumer change only. The
frontend already hides the facet when the list is empty, so B removes the visible dead control without a frontend
change; the URL `rating` param becomes an error path and should be removed in a frontend follow-up.
B is a behavior change for a client that sends `min_rating > 0` (it gets an error instead of an empty page). That is
intentional: an empty page is indistinguishable from "nothing matches". Flagged as Open Question 2.

### D9 - Sequencing and conflict points with the two in-flight changes

Fixed order: `search-stock-events` first (on `fix/repo-doctor-search`), this change stacks on it, `add-hybrid-retrieval-platform`
afterwards. The team-search working tree already shows stock-events edits to `internal/index/opensearch.go` and an
untracked `internal/index/integration_test.go`; implementation of this change starts from that branch tip, not `main`.

| Shared place | search-stock-events | this change | Resolution |
|---|---|---|---|
| `EnsureIndex` / mapping const | put-mapping `stock`, `stock_version` (D4) | add `created_at` | one put-mapping body with all fields; one conflicting-mapping failure path |
| `Upsert` carry-forward | GET existing `stock`/`stock_version` (D3) | also carry `created_at` | extend the same GET and `ListingDoc` copy; no second read |
| scripted updates | `UpdateStock` guard on `stock_version` | tombstone noop line | add the line to `PartialUpdate`, `UpdateStock`, `SetCreatedAt`; stock tests gain a tombstone case |
| filter builder (stock task 2.4) | `in_stock` -> `range stock > 0` | default `status=published` | both live in the one builder `Search` already calls; `Suggest` gets the status clause directly |
| `Index` interface + fakes | adds `UpdateStock`, `Hit.Stock` | changes `Delete` to `Delete(ctx,id,version)`, adds `SetCreatedAt`, `PurgeDeleted` | one pass over the fakes per change; mechanical |
| `ListingEventHandler` | new `ListingStockChanged` case | delete branches + `CREATED` | disjoint lines |
| stock scenarios "deleted listing stays absent" | asserts absence | tombstone doc exists | stock's tests assert "no published document in search", not "GET by id is 404" (task in track 2) |

`add-hybrid-retrieval-platform` (0/39, alias-addressed `listings-v{N}`, v2 `knn_vector` mapping, replay-based rebuild)
then inherits, and must be amended (a task here mirrors stock's task 6.1): its v2 mapping and startup contract check
include `created_at` next to `stock`/`stock_version`; its strategies build filters only through the shared builder
(so the default-published rule and `in_stock` apply to semantic and structured retrieval too); its replay reproduces
tombstones without extra work because they are ordinary documents; its `Engine` calls the same handler-level policy
before strategies run. None of this order is load-bearing for correctness of the others, but doing hybrid first would
make its new strategy paths the ones most likely to forget the status default.

## Risks / Trade-offs

- [Clients that relied on drafts leaking, or on an empty page for `min_rating`, now get results removed or an error]
  -> Intended. The frontend already sends `status=published` and no `min_rating` unless the URL carries `rating`; the
  e2e suite is checked in track 2. Release note in the README.
- [Tombstones stay visible to an operator query and count in `_count`] -> They are only reachable by direct index
  access; every service path filters them; the purge bounds them. The README and ADR addendum say so.
- [A full upsert newer than a delete revives it] -> By design (external versioning); the domain never emits an update
  after a delete. A skewed clock could order an old update after a delete; the same assumption already underpins all
  versions. Documented.
- [`created_at` is missing on existing documents until replay] -> They sort last under `SORT_BY_NEWEST` (defined
  behavior, spec). Replay via the runbook stock adds (its task 8.3) fills it; a `SetCreatedAt`-only replay works even
  against a live index because the update is independent of the external version.
- [`Delete` now writes instead of removing, so a delete storm costs index writes] -> One small document per deleted
  listing, same refresh behavior as before; purge removes them in bulk. Low-volume event type.
- [Arbitrary `filters` keys still become `term` clauses on any field] -> Out of scope beyond `status`, but `status`
  handling is the only place where a caller-controlled key decides visibility; a follow-up may add an allow-list.
- [pgx + `database/sql` adds a dependency and a connection pool to a service that had none] -> Same version as two
  sibling services; pool size is configurable; no pool when `DATABASE_ENABLED=false` in local/test.
- [Migration for the k8s path is not solved here] -> `platform-gitops` has no migration hook for any service today;
  staging/production for `team-search` therefore still need an operator-run or chart-level `migrate up` before the
  guard lets the server boot. Flagged, same gap as the siblings, not widened here.
- [The 14-day retention is a guess] -> Configurable; chosen to exceed redelivery/DLQ re-drive windows. A smaller value
  only weakens protection for events delayed longer than it.

## Migration Plan

1. Merge `search-stock-events` (mapping + carry-forward + stock scripts) on `fix/repo-doctor-search`.
2. Land this change on top: add `team-search-migrate` to compose first (harmless: `search_db` exists; `migrate up`
   creates `saved_searches`), then the `team-search` image with the `DATABASE_*` config. Until a database is
   configured, `local` keeps running in memory with a warning; set `DATABASE_ENABLED=true` and a URL wherever durability
   matters.
3. Deploy indexer and query server together: both call `EnsureIndex` (idempotent put-mapping adds `created_at`).
   Order between them does not matter.
4. Backfill `created_at` and tombstones: replay `listing.events` per the runbook (stop indexer, reset the consumer
   group to earliest or use a scratch index, restart). `CREATED` events set `created_at`; delete events create
   tombstones. No replay is required for correctness of published-only, ownership or durability.
5. Existing saved searches (in-process only) are unrecoverable by nature; nothing to migrate.
6. Rollback: revert the image. `created_at` stays in the mapping and documents (unread by old code); tombstone
   documents with status `deleted` would be **returned by old code** (old `Search` has no status filter), so roll back
   only after a purge, or keep the default-published filter (D1) as a separate revertable commit that is last to go.
   The saved-search table and rows are left in place; old code ignores them.

## Open Questions

1. **Draft owner view, keep or drop?** Recommendation: keep (D2), it is one policy function. If the answer is "sellers
   use `ListMyListings`, drop it", the spec loses two requirement scenarios and D2 shrinks to "draft is never returned".
   This changes behavior, so it should be confirmed before implementation.
2. **Rating: B (hide + reject) vs A (populate).** Recommendation: B now (D8). Choosing A changes scope: it needs a
   `platform-core` event contract and a `team-engagement` producer and is its own change. Confirm B, or confirm A as a
   follow-up change and keep B as the interim.
3. **Creation time source.** Recommendation: event `occurred_at` (D3). Needs no contract change. If product wants
   exact creation times for listings that predate the topic's retention, the only fix is a `created_at` field on
   `Listing` from `team-domain` (a platform-core proto change); confirm that is not wanted now.
