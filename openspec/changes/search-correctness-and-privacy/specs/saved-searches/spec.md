## Purpose

Defines saved searches as durable, private, per-user state of `team-search`: who may use them, that one user can never
read or run another's, that they survive restarts and replicas, and that staging and production never run on
in-memory storage.

## ADDED Requirements

### Requirement: Saved-search RPCs require an authenticated user principal

`SaveSearch`, `ListSavedSearches`, `DeleteSavedSearch` and `RunSavedSearch` SHALL require a principal of type `user`
with a non-empty id that is not the reserved anonymous id. A call with no principal, or with an anonymous principal,
SHALL fail with `UNAUTHENTICATED`. A call with an authenticated principal of any other type (for example `service`)
SHALL fail with `PERMISSION_DENIED`. This identity check SHALL run before the scope check, so the outcome does not
depend on which scopes the gateway grants anonymous callers, and it SHALL hold even if an anonymous caller is granted
`search:read`. The existing scope requirements (`search:read` for list and run, `search:write` for save and delete)
SHALL continue to apply after the identity check. No saved-search RPC SHALL ever execute against a shared or default
owner.

#### Scenario: Anonymous cannot list saved searches

- **WHEN** an anonymous caller (principal type `anonymous`, id `anonymous`, scope `search:read`) calls `ListSavedSearches`
- **THEN** the call fails with `UNAUTHENTICATED` and returns no saved searches

#### Scenario: Anonymous cannot run a saved search

- **WHEN** an anonymous caller with scope `search:read` calls `RunSavedSearch` with the id of an existing saved search
- **THEN** the call fails with `UNAUTHENTICATED` and runs no query

#### Scenario: Anonymous cannot save or delete

- **WHEN** an anonymous caller calls `SaveSearch` or `DeleteSavedSearch`, even if granted `search:write`
- **THEN** the call fails with `UNAUTHENTICATED` and nothing is stored or removed

#### Scenario: A call with no principal is unauthenticated

- **WHEN** any saved-search RPC is called without forwarded principal metadata
- **THEN** it fails with `UNAUTHENTICATED`

#### Scenario: A service principal is denied

- **WHEN** a principal of type `service` with scopes `search:read` and `search:write` calls any saved-search RPC
- **THEN** the call fails with `PERMISSION_DENIED`

#### Scenario: A signed-in user with the right scopes succeeds

- **WHEN** a `user` principal with `search:read` and `search:write` saves a search and then lists
- **THEN** the save succeeds and the list contains it

### Requirement: Every saved-search operation is scoped to its owner

Saved searches SHALL be created, listed, fetched, run and deleted only within the calling user's own set. A request
naming the id of another user's saved search SHALL behave exactly as a request for an id that does not exist
(`NOT_FOUND`), so ownership cannot be probed. `ListSavedSearches` SHALL return only the caller's saved searches and a
`page.total` that counts only them. `RunSavedSearch` SHALL apply the same listing-visibility rules as `SearchListings`
using the runner's own identity: a saved `status=draft` filter SHALL run only for an owner whose saved `seller_id`
filter equals their own id, and SHALL otherwise fail as `SearchListings` would. `SaveSearch` SHALL reject, with the same
errors, a filter set that `SearchListings` would reject for the caller.

#### Scenario: A user cannot see another user's saved searches

- **WHEN** user A saves a search and user B calls `ListSavedSearches`
- **THEN** B's response is empty with `page.total` 0

#### Scenario: A user cannot run another user's saved search

- **WHEN** user B calls `RunSavedSearch` with the id of user A's saved search
- **THEN** the call fails with `NOT_FOUND` and runs no query

#### Scenario: A user cannot delete another user's saved search

- **WHEN** user B calls `DeleteSavedSearch` with the id of user A's saved search
- **THEN** the call fails with `NOT_FOUND` and A's saved search still exists

#### Scenario: Running a saved search applies the published-only rule

- **WHEN** user A runs a saved search whose query matches both a published and a draft listing owned by other sellers
- **THEN** the result contains the published listing and not the draft

#### Scenario: A saved draft filter is rejected for a non-owner

- **WHEN** user B tries to save a search with `filters_json {"status":"draft"}` and no `seller_id` equal to B
- **THEN** the call fails with `PERMISSION_DENIED` and nothing is stored

### Requirement: Saved searches are durable across restarts and replicas

When the service runs with a database, saved searches SHALL be stored in `team-search`'s own Postgres database, be
visible to every replica, and survive a restart of the query server. The schema SHALL be applied by the same
migration mechanism the other services use (a golang-migrate step before the server starts), and the migration SHALL be
idempotent. The in-memory repository SHALL be permitted only when the runtime environment allows it (next
requirement).

#### Scenario: A saved search survives a restart

- **WHEN** a signed-in user saves a search, the `team-search` query server is restarted, and the same user lists
  saved searches
- **THEN** the saved search is listed with the same id, query and filters

#### Scenario: A saved search is visible across replicas

- **WHEN** a signed-in user saves a search through one query-server replica and lists through another
- **THEN** the saved search is listed

#### Scenario: Migration is repeatable

- **WHEN** the migration step runs twice against the same database
- **THEN** the second run changes nothing and reports success

### Requirement: Staging and production refuse in-memory saved-search storage

The query server SHALL fail to start, before it accepts traffic, when `ENV` is `staging`, `stage`, `prod` or
`production` (case-insensitive, trimmed) and the database is disabled or unreachable. In every other environment
(`local`, `test`, unset or unknown) it MAY run on the in-memory repository, and it SHALL log at warn level that saved
searches are not durable. The Kafka indexer SHALL NOT require a database. The failure message SHALL name `ENV`,
`DATABASE_ENABLED` and `DATABASE_URL`.

#### Scenario: Production without a database does not boot

- **WHEN** the query server starts with `ENV=production` and `DATABASE_ENABLED=false`
- **THEN** it exits with an error naming `DATABASE_ENABLED` before opening its gRPC port

#### Scenario: Staging with an unreachable database does not boot

- **WHEN** the query server starts with `ENV=staging`, `DATABASE_ENABLED=true` and a `DATABASE_URL` that cannot be reached
- **THEN** it exits with an error before opening its gRPC port

#### Scenario: Local runs in memory and says so

- **WHEN** the query server starts with `ENV=local` and `DATABASE_ENABLED=false`
- **THEN** it serves requests using in-memory saved searches and logs a warning that they are not durable

#### Scenario: The indexer does not need a database

- **WHEN** the indexer starts with `ENV=production` and no `DATABASE_URL`
- **THEN** it starts normally
