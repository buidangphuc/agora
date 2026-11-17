## Context

Admin RPCs are gated at the edge by `adminProcedures` and audit-first (AGENTS.md section 3.6). The seller needs a
parallel surface that scopes by principal. Contracts come from `seller-centre-contract`; nothing here edits proto.

## Decisions

- New RPCs, not an opened edge policy. Seller handlers use `auth.RequireScopes(ctx, "listing.write")` (not
  `requireSeller`, which also admits `admin`; an admin holding only `promotion.admin` has no shop to manage).
  The seller id is `principal.ID`.
- Ownership as the query filter. `ListMyCampaigns` filters `seller_id = $principal`. Cancel/deactivate use one
  statement or locked transaction keyed by id and owner; zero rows matched maps to PERMISSION_DENIED (new sentinel
  `ErrNotOwner` mapped in `errTable`; message identical for foreign and unknown ids, so no existence oracle).
- Cancel: new repository method `CancelIfScheduledOwned(ctx, id, sellerID, now)` locking the row
  (`lockCampaignState`), checking in order: missing or `seller_id` mismatch -> not owner; already cancelled ->
  return existing time; started -> `ErrCampaignNotScheduled` (FAILED_PRECONDITION, ended included); else set
  `cancelled_at`. The admin `Cancel` path and its audit are untouched.
- Voucher deactivate: `GetByID`, check scope SHOP, `seller_id` = caller and no `owner_user_id`, then the existing
  `Deactivate` (idempotent `COALESCE(deactivated_at, ...)`). No `auditx.Do` on this path.
- Voucher list: reuse `repo.List(sellerID, ...)` (already excludes user-bound rows and includes inactive ones);
  add a scope = SHOP condition so platform rows stored under an empty or admin seller id never leak.
- `seller_id` on create: the seller path already calls `listing.Get`; store `l.GetSellerId()`. The admin path
  without an idempotency key skips the lookup today; it now looks the listing up best-effort only to fill
  `seller_id` (a failed lookup leaves `''` for the backfill and does not fail the admin call), keeping admin
  behaviour unchanged.
- Backfill is a subcommand in `internal/app/execute.go` (`len(args)==1`), using the same `UPSTREAM_DOMAIN_ADDR`
  listing client and a service principal; keyset batches `WHERE seller_id = '' AND id > $last ORDER BY id LIMIT 100`.
- Gateway: four forwarder methods copying `CreateVoucher`/`ListCampaigns` shape (`callRead` for lists, `callWrite`
  for stops, `promotionErr`). Router already mounts both forwarders, so no wiring change expected.

## Risks

- `seller_id = ''` rows invisible until backfill runs: the backfill must run once per environment after migrate
  (local first; staging/prod are out of scope).
- Cancel is stricter than admin cancel (SCHEDULED only); intentional per the locked decision.
