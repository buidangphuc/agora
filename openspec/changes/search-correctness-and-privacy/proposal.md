## Why

Six defects in `team-search`, each verified in code on branch `fix/repo-doctor-search`, make the search product return
the wrong things or expose one user's data to another:

1. **Drafts are searchable.** `OpenSearchIndex.Search` adds only the filters the caller sends. With no
   `{"status":"published"}` filter, DRAFT listings come back, and `Suggest` has no filter at all, so draft titles are
   suggested to everyone. `FEATURES.yaml` and the README promise *published* listings. The team-frontend always sends
   `status=published`, which hides the bug from the UI and from the existing e2e; a direct gateway call or any other
   client sees drafts.
2. **"Newest" is not newest.** `SORT_BY_NEWEST` sorts by the document `_id` descending. Ids are not time-ordered, and
   the index holds no creation time at all (`Listing` carries no timestamp; only the event envelope has `occurred_at`).
3. **Saved searches are not durable.** `cmd/server/main.go` always wires `NewInMemorySavedSearchRepository()`. The
   Postgres repository and `migrations/0001_saved_searches.*.sql` exist but nothing opens a pool or applies the
   migration; `internal/config` has no `DATABASE_*` fields even though `docker-compose.services.yaml` already passes
   `DATABASE_ENABLED` / `DATABASE_URL` to `team-search` (silently ignored). A restart, or a second replica, loses or
   splits every user's saved searches. There is no ENV guard against this, unlike `team-order`.
4. **Saved searches are not private.** `callerID` only checks that the principal id is non-empty. The gateway forwards an
   unauthenticated caller as id `anonymous`, type `anonymous`, scopes `listing.read,search:read`
   (`team-gateway/internal/edge/forward.go`, `PUBLIC_SCOPES`). `ListSavedSearches` and `RunSavedSearch` only need
   `search:read`, so **every anonymous visitor shares one saved-search bucket** and can list and run whatever any other
   anonymous caller (or a misrouted write) put there. `SaveSearch` / `DeleteSavedSearch` are blocked only by the missing
   `search:write` scope, which is an accident of `PUBLIC_SCOPES`, not a rule.
5. **A deleted listing can come back.** `Delete` removes the document and writes no version tombstone. OpenSearch keeps
   its own delete-tombstone for a short, cluster-configured window (`index.gc_deletes`, 60s by default), after which a
   late or replayed stale `ListingChanged` upsert is accepted as brand new and resurrects the listing.
6. **Rating is a dead feature.** No handler ever writes `rating` (`ListingDoc.Rating` is always 0, and no listing event
   carries a rating), so `min_rating > 0` always matches nothing and the `ratings` facet always reports four buckets of
   count 0. A full upsert also writes `rating: 0`, which would clobber any future source. The team-frontend renders the
   ratings facet whenever the list is non-empty, so a dead control is shown to buyers today.

## What Changes

- **team-search: visibility.** Search and suggest return only `published` listings by default, enforced in the shared
  index filter construction so every caller (handler, saved-search run, later retrieval strategies) inherits it. A
  caller may see `draft` listings only as their own: an authenticated **user** principal whose `seller_id` filter equals
  their own id and who asks for `status=draft` explicitly. Any other use of a non-published `status` is rejected
  (`PERMISSION_DENIED` for draft, `INVALID_ARGUMENT` for unknown values). Additive: no proto change.
- **team-search: newest.** The index gains `created_at` (from the `CREATED` `ListingChanged` event's `occurred_at`),
  added to the live mapping by an idempotent put-mapping, preserved across full upserts, and used as the
  `SORT_BY_NEWEST` key with a deterministic tie-break. Listings not yet carrying it sort last until replay.
- **team-search: saved-search storage.** Config gains `DATABASE_ENABLED` / `DATABASE_URL`; `cmd/server` opens a
  Postgres pool and wires `PostgresSavedSearchRepository`; migration `0001` is applied by a golang-migrate init step
  exactly like the other services (`team-search-migrate` in `docker-compose.services.yaml`). The in-memory
  repository remains for `local`/`test` only, behind the `team-order` boot-guard pattern: `ENV` in
  `staging|stage|prod|production` refuses to start without a working database.
- **team-search: saved-search privacy.** Every saved-search RPC requires an authenticated **user** principal. No
  principal or an anonymous principal gives `UNAUTHENTICATED`; an authenticated non-user (service) principal gives
  `PERMISSION_DENIED`. The principal check runs before the scope check so the outcome does not depend on
  `PUBLIC_SCOPES`. Every repository call stays scoped by owner; cross-owner access is indistinguishable from
  not-found. `RunSavedSearch` re-applies the visibility rule with the runner's identity.
- **team-search: delete tombstones.** `Delete` becomes a **version-guarded soft delete**: it indexes a minimal
  `status=deleted` document under the event's external version, so any event at or before the delete is rejected by
  the same guard that orders every other write (AD2). Partial updates and stock updates treat a tombstone as sticky.
  Search and suggest never match it. Tombstones are purged by an indexer-side job after a configurable TTL (default 14
  days).
- **team-search: rating honesty (recommended option, see Open Questions in design).** Until a rating source exists,
  `min_rating > 0` is rejected with `INVALID_ARGUMENT`, the `ratings` facet is returned empty, and a full upsert stops
  writing `rating`. The mapping field stays so a future source needs no mapping change.
- **team-gateway / team-frontend: follow-ups only, no required change** (listed in tasks): optional early reject of
  anonymous saved-search calls at the edge; the frontend stops sending `rating` and hides the control (it already
  hides an empty facet list).
- **platform-core: no proto change.** One ADR-0005 addendum. Nothing here needs a new field, RPC or event.

## Sequencing with in-flight changes

This change **stacks on `search-stock-events`**, which is implemented first on the `team-search`
`fix/repo-doctor-search` branch. Both touch `internal/index/opensearch.go` and `internal/consumer/listing.go`, so the
order is fixed and the overlaps are explicit (details in design D9):

- `EnsureIndex`: stock adds an idempotent put-mapping for `stock`/`stock_version`; this change extends the **same**
  put-mapping call with `created_at`.
- `Upsert`: stock adds a read-before-write carry-forward of `stock`/`stock_version`; this change carries `created_at`
  through the **same** GET.
- The scripted updates (`PartialUpdate`, stock's `UpdateStock`) gain one extra noop condition for a tombstone.
- The shared filter builder introduced by stock task 2.4 (`in_stock`) is where the default `status=published` lives.
- `ListingEventHandler`: stock adds a `ListingStockChanged` case; this change changes the three delete branches and the
  `CREATED` branch only. No shared lines.
- stock's "deleted listing stays absent" scenarios remain true at the search-result level. A test that asserts the
  document itself is absent (404) must assert "no published document" instead, because a tombstone document now exists.

`add-hybrid-retrieval-platform` lands **after** both. It inherits all of this: its v2 mapping must include
`created_at` (and keep `status`), its strategies must build filters through the shared builder, and because the
tombstone is an ordinary versioned document, its replay-based alias flip rebuilds tombstones for free.

## Non-goals

- Hybrid/semantic retrieval itself (`add-hybrid-retrieval-platform`).
- Stock in the index (`search-stock-events`).
- Ranking or relevance changes; `SORT_BY_RELEVANCE` and price sorts are untouched.
- Any UI work (the frontend items are follow-ups, not part of this change).
- Adding a rating source (a review event, or polling `team-engagement`). Only the honest behavior until one exists.
- Sharing saved searches between users, or admin access to drafts.
- Changing the `SearchService` proto, or how the gateway resolves identity.

## Capabilities

### New Capabilities
- `search-query-correctness`: what a search or suggest call may return: published-only by default, the owner-only draft
  view, creation-time ordering of "newest", and the honest behavior of the rating filter and facet.
- `saved-searches`: durable, per-user saved searches: authenticated-user-only access, owner scoping on every path,
  persistence across restart and replicas, and the boot guard against in-memory storage in staging/production.
- `search-read-model-deletes`: a deleted listing stays deleted: version-guarded tombstones, stale-event immunity, and
  tombstone retention.

### Modified Capabilities

_None._ `openspec/specs/` has no search capability yet (the in-flight `search-stock-read-model` and `search-retrieval`
are unarchived new capabilities), so there is no existing requirement to delta.

## Impact

**`team-search`** (all of the code): `internal/index/opensearch.go` (filter builder, sort, mapping, `Delete`,
scripted-update guards, `SetCreatedAt`, purge), `internal/consumer/listing.go` (delete and created paths),
`internal/handler/search.go` + `saved_search.go` (visibility policy, principal check, `min_rating`),
`internal/config` (`DATABASE_*`, tombstone TTL, boot guard), `internal/bootstrap` + `cmd/server/main.go` +
`cmd/indexer/main.go` (pool, repository choice, purge loop), `internal/repository` (tests, Postgres driver wiring),
`go.mod` (pgx v5, the same `v5.6.0` the sibling services pin), `.env.example` (drift gate), `README.md`,
`FEATURES.yaml`, `Makefile` (throwaway-Postgres integration target).

**Repo root** (`full_team_repo`): `docker-compose.services.yaml` gains `team-search-migrate` and a `depends_on` for
`team-search`; `team-search-indexer` needs no database.

**`platform-e2e`**: new API-level scenarios (draft not in public search, anonymous cannot use saved searches, saved
searches survive a team-search restart, a deleted listing stays deleted after a stale event replay). The existing
logged-in-buyer saved-search UI scenarios must stay green.

**`platform-core`**: ADR-0005 addendum only. **`platform-gitops`**: a note that no service has a migration hook there
today; `team-search`'s chart needs `DATABASE_*` and the migration run before the saved-search store can be durable in
k8s (flagged, not done here).

**Operational**: existing documents have no `created_at` until a replay of `listing.events`; existing deletes have no
tombstone, which is harmless (they simply predate the guard). Saved searches created before this change lived only in a
process and cannot be recovered.
