# Design

## Write path (service)

### Requested status (handler)

`domainStatus` maps `LISTING_STATUS_UNSPECIFIED` to `""` (status omitted), not to `draft`, so the service can tell "status omitted" from `DRAFT` (decision A8). `DRAFT` maps to `draft`, `PUBLISHED` to `published`; every server-only value (`REJECTED`, `PENDING_REVIEW`, `BANNED`, `HELD`) is passed to the service as `""` too, so a seller-sent server-only status behaves exactly like an omitted one. This change ships in task 1.

### `resolveStatus(existing, requested, callerIsAdmin, callerIsOwner, contentChanged)`

`CreateWithEvent` / `UpdateWithEvent` resolve the stored status in this one function. Rules are checked in order; the first match wins.

1. Admin caller who is not the owner (update only): status kept (`existing.Status`). No screen. An admin's own listing (create, or update of a listing they own) does not match this rule and follows the seller rules below (decision A12); the Q3 `listing.admin_update` audit event is still written for every admin update.
2. Existing `BANNED` / `HELD`: `ErrStatusLocked` (handler: `FAILED_PRECONDITION`).
3. Existing `REJECTED`: `PENDING_REVIEW` (the screen runs only to record flags; its verdict cannot publish). Task 1 interim: stays `REJECTED`.
4. Existing `PENDING_REVIEW`: stays `PENDING_REVIEW` whatever is sent (content edits re-screen to refresh flags). Withdrawing a pending listing to draft is not offered.
5. Requested `draft` (create, or existing `DRAFT` / `PUBLISHED`): `DRAFT`. No screen.
6. Existing `PUBLISHED` with a content change and any requested value other than `draft` (`published`, omitted, or a server-only value): screen; `ALLOW` and no keyword hit gives `PUBLISHED`, else `PENDING_REVIEW` (decision A8; closes abuse case 2).
7. Existing `PUBLISHED` without a content change: `PUBLISHED`. No screen.
8. Requested `published` on create or on an existing `DRAFT`: screen; `ALLOW` and no keyword hit gives `PUBLISHED`, else `PENDING_REVIEW`.
9. Otherwise (requested omitted on create or on an existing `DRAFT`): `DRAFT`.

Task 1 interim (before the screen and the new statuses exist): rules 6 and 8 store `PUBLISHED` without a screen; rule 9 keeps the existing `DRAFT`; an omitted status on an existing `PUBLISHED` listing keeps it `PUBLISHED`.

Content change = title, description, category id or image keys differ from the stored row.

The screen runs before the transaction; the transaction (existing `CreateTx` / `UpdateTx`) persists status, moderation fields and the `ListingChanged` row together. `submitted_at` is set whenever the new status is `pending_review` and was not before.

### Seller delete

`DeleteWithEvent` by the owner (including an admin owner) of a `BANNED` or `HELD` listing returns `ErrStatusLocked` (`FAILED_PRECONDITION`), writes no outbox row and leaves the row in place (decision A10). An admin deleting another seller's listing is unchanged.

### Concurrency

- `UpdateTx` for a seller write is conditional: `UPDATE listings ... WHERE id = $1 AND status = $loaded_status AND updated_at = $loaded_updated_at`. Zero rows affected returns `ErrStatusChanged` (handler: `FAILED_PRECONDITION`, "listing changed, reload and retry") and no outbox row, so a seller write cannot overwrite a moderation decision taken after the seller's read.
- `ModerateListing` uses a new `TxWriter.ModerateTx(id, fromStatus, loadedUpdatedAt, ...)` that updates status + moderation columns with the same condition (`WHERE id = $1 AND status = $from_status AND updated_at = $loaded_updated_at`) and enqueues the outbox row in the same transaction. Zero rows affected returns `ErrStatusChanged` (`FAILED_PRECONDITION`) with no status change and no outbox row. So of two admins deciding at once, or an admin deciding while the seller edits, exactly one change is applied.
- The audit-then-transaction order matches the Q3 admin path: the losing `ModerateListing` call has already written its `listing.moderate` event (audit is append-only), so team-audit can hold one event per attempt; the listing row and the outbox hold exactly one change.

## ModerateListing order

`RequireScopes("admin")` -> validate the request (decision, reason code, note length; `INVALID_ARGUMENT`) -> load (`NOT_FOUND`) -> transition check (`FAILED_PRECONDITION`) -> team-audit `listing.moderate` (fail closed, `UNAVAILABLE`) -> `ModerateTx`. Validation comes before the load, so an unknown id with a bad reason code returns `INVALID_ARGUMENT`. Neither the validation nor the load writes an audit event.

## Moderation visibility

- `Listing.moderation` on reads: an admin or a service principal sees `reason_code`, `note`, `screen_flags`, `decided_at`; the owner sees `reason_code`, `note` and `decided_at`, and `screen_flags` is empty (decision A11, so a seller cannot learn which word tripped the screen); everyone else gets an empty `moderation`.
- `ListingChanged` outbox payloads (built in `handler/listing.go` `enqueueListingChanged`) carry `moderation.reason_code` and `decided_at` but always an empty `note` and empty `screen_flags` (decision A13).
- The contract comment at `00-spec.md` (`screen_flags` "admin/owner only") is owned by `q6-trust-contract`; this slice implements admin-only.

## Prohibited-term list

The default list is embedded in the binary with `go:embed` (`internal/screen/prohibited_terms.txt`), so the distroless image (`team-domain/Dockerfile` copies only `/server`) needs no extra file and no Dockerfile change. `SCREEN_TERMS_FILE` (default empty) overrides it with a file path; when it is set and the file is missing, start-up fails. The seeded list is a minimal one until a named legal owner signs off before launch.

## Assumptions (no human decision yet)

- E: the mock screen fixture marker is defined by `ai-listing-screen`; e2e uses it by name from that slice's helper. The mock default ALLOWs clean text (cross-slice fix), so existing e2e seeds that publish clean listings still auto-publish.
- F: `UPSTREAM_AI_ADDR` for team-domain in the root compose (`docker-compose.services.yaml`) is not edited here; if unset every publish goes to review (fail safe) and existing e2e seeds that publish directly would stall. The e2e track adds the variable for team-domain in the e2e overlay if the contract slice has not.

## Gateway

`ModerateListing` is a write forward (`callWrite`); `ListModerationQueue` a read (`callRead`). No new edge rate-limit class.

## Clarifications

### Round 1 — 2026-10-16

- A8 [authz] Status ownership: Content edit with status omitted is re-screened: is every seller content edit of a `PUBLISHED` listing re-screened, and can the service tell an omitted status from `DRAFT`? → **Yes: every content edit of a `PUBLISHED` listing is re-screened whatever status is sent, a flagged edit moves it to `PENDING_REVIEW`, and `domainStatus` maps UNSPECIFIED to "omitted", not `draft`** (human)
- A9 [state] Moderation: Ban takes down a published listing: may an admin REJECT/BAN a `PUBLISHED` listing, and may APPROVE reverse a `REJECTED`/`BANNED` one? → **REJECT/BAN allowed from `PENDING_REVIEW` and `PUBLISHED`; no reversal, APPROVE of `REJECTED`/`BANNED` is refused** (human)
- A10 [authz] Status ownership: Banned or held listing refuses seller delete: may a seller delete a `BANNED`/`HELD` listing? → **No: `FAILED_PRECONDITION`** (human)
- A11 [privacy] Status ownership: Owner sees status and reason but not flags: who sees `screen_flags`? → **Admins (and service principals) only; the owner sees `reason_code` and `note` only** (human)
- A12 [authz] Status ownership: Admin's own listing goes through the screen: how is an admin's own listing published? → **It follows the seller path (screen runs) and is still audited** (human)
- A13 [privacy] Moderation: ListingChanged carries no note or flags: do `ListingChanged` events carry the moderation note and flags? → **No: the event carries neither the note nor the screen flags** (human)
- Cross-slice [e2e] Auto-screen: Publish end to end, clean and flagged: does the mock screen allow clean text? → **Yes, the mock default ALLOWs clean text so existing e2e seeds still auto-publish (owned by `ai-listing-screen`)** (human)
- NEEDS_HUMAN [legal] Auto-screen: Keyword list loads from the config file: who signs off the prohibited-term list? → **A named legal owner signs it before launch; a minimal seeded list ships until then** (human)
