## Why

`flash_sale_campaigns.stock_sold` is never written, so `GetFlashSaleStock.remaining` always equals `stock_cap` and nothing stops a flash sale from selling more than its cap (`team-promotion/README.md`, Known gaps). The initiative `q5-advertised-price` (Locked 2026-10-16) makes team-order charge the flash price and requires the flash stock to be capped by a hold, like the voucher hold (ADR-0008). This slice is the team-promotion side: it implements the three service-only `FlashSaleService` RPCs `ReserveFlashSaleStock`, `CommitFlashSaleStock` and `ReleaseFlashSaleStock` (declared by `q5-flash-stock-contract`) and makes the stock meter count holds. team-order calls them in slice `order-advertised-price`.

initiative: q5-advertised-price

## What Changes

- **Migration `0004_flash_sale_holds`** (up and down): new table `flash_sale_holds` (`id`, `reservation_id` UNIQUE, `campaign_id` FK to `flash_sale_campaigns` ON DELETE CASCADE, `listing_id`, `variant_id`, `quantity`, `status` reserved|committed|released, `expires_at` = created + 15 min, timestamps) and an index on `(campaign_id, status)`. Mirrors `voucher_reservations`.
- **Repository** (`internal/repository/flashsale_hold.go`, new, mirrors `reservation.go`): `FlashSaleHoldRepository` with Postgres and in-memory implementations:
  - `Reserve`: one transaction that locks the campaign row (`SELECT ... FOR UPDATE`), re-reads `stock_sold` and the sum of `reserved` holds, and inserts the hold only if `stock_cap - stock_sold - holds >= quantity`; idempotent on `reservation_id` (the existing row is returned, no second hold).
  - `Commit`: one transaction, locks the campaign row, moves an active (`reserved`, unexpired) hold to `committed` and adds `quantity` to `stock_sold` exactly once; an expired hold is refused.
  - `Release`: moves an active hold to `released`; moves a `committed` hold to `released` and subtracts `quantity` from `stock_sold` in the same transaction (once); an already released or expired hold is a no-op returning `released=true`.
  - `HeldQuantity(campaignID, now)` returns the sum of active unexpired holds, used by the stock meter. Expiry is evaluated at read (`expires_at > now`), so no sweep is needed for correctness; a purge deletes released and expired rows (below).
  - The in-memory implementation takes one mutex around check-and-insert so unit tests exercise the same invariant.
- **Service** (`internal/service/flashsale.go`): `ReserveFlashSaleStock`, `CommitFlashSaleStock`, `ReleaseFlashSaleStock`; `GetFlashSaleStock` now returns `remaining = max(0, stock_cap - stock_sold - active unexpired holds)`. Reserve refusals (`reserved=false` with a `reason`, not a gRPC error) use exactly the contract set, checked in order, first failure wins: `not_active` (campaign unknown, outside `[starts_at, ends_at)`, kill-switch `FlagFlashSaleEnabled` OFF, listing differs, or the campaign has a `variant_id` that differs from the request; also a replay of a released or expired hold), `price_changed` (`expected_sale_price != sale_price`), `insufficient_stock`. Input errors (`INVALID_ARGUMENT`): empty `reservation_id`, `campaign_id` or `listing_id`, `reservation_id` over 200 bytes, `quantity <= 0`; empty `variant_id` is valid.
- **Handler** (`internal/handler/flashsale.go`): the three handlers call `interceptor.RequireService(ctx, ScopePromoReserve)` (existing scope `promotion.reserve`, held only by `service-team-order`); a user principal that carries the scope is still `PERMISSION_DENIED`; anonymous is `UNAUTHENTICATED`. `reservation_id` empty, `campaign_id` empty (reserve) or `quantity <= 0` is `INVALID_ARGUMENT`.
- **Wiring**: `cmd/server/main.go` and `internal/bootstrap/resources.go` build the hold repository (Postgres or in-memory by `DATABASE_ENABLED`) and pass it to `FlashSaleService`. Generated Go from the proto copy that the contract slice vendors under `team-promotion/proto/` (`make proto`, not hand-edited).
- **Purge**: a daily ticker in `cmd/server/main.go` deletes only `released` holds and expired uncommitted holds (`reserved` with `expires_at` older than now) more than 30 days after they became terminal (`PurgeReleasedAndExpiredHolds`). Active and `committed` holds are never purged; committed holds stay until the campaign is deleted (cascade), so a later `CancelOrder` release of an old placed order still returns its units.
- **README**: the Known gaps line "`flash_sale_campaigns.stock_sold` is never incremented" is removed and a short "Flash stock holds" section is added.
- **platform-e2e**: new feature `features/promo/flash_sale_stock_hold.feature` + step bindings + helper `support/flash_hold_support.py` (service-principal gRPC calls, see `design.md`). Campaigns are seeded through `FlashSaleService.CreateCampaign` as admin (the audit upstream is already wired in the compose file). `FEATURES.yaml` is written later by the e2e-author.
- Not changed: proto (contract slice), gateway policy (the RPCs are not routed to browsers), `GetActiveFlashSale`, `CreateCampaign`, voucher code, team-order.

## NFR

- Latency: each hold RPC is one DB transaction with one row lock on a single campaign; non-gated target only, no scenario or test asserts it: p95 under 50 ms at 100 concurrent reserves on one campaign in the local stack. Concurrent reserves for the same campaign serialise on the row lock (flash-sale hot row accepted: throughput of one campaign is bounded by transaction time).
- Correctness: at no point does `stock_sold + sum(reserved holds)` exceed `stock_cap` through these RPCs; a retried reserve, commit or release never changes stock twice.
- Availability: no new upstream dependency. Kill-switch evaluation fails open as today (flag provider error = sale enabled), so the reserve path stays available when Flipt is down.
- Limits: `quantity` is an int64 > 0; `reservation_id` at most 200 bytes; a request above `stock_cap` simply fails the remaining check. Holds expire 15 minutes after reserve (same as the team-domain reservation TTL).

## Data classification

| Field | Class | Purpose | Consent basis | Retention |
|---|---|---|---|---|
| `reservation_id` (order id + listing/variant) | business identifier (links to an order, indirectly to a buyer) | idempotency key of the hold | contract performance (order processing) | kept with the row; released and expired rows are deleted 30 days after they became terminal by the daily purge; committed rows are kept until the campaign is deleted (cascade) so a late cancel can still release them |
| `campaign_id`, `listing_id`, `variant_id`, `quantity`, `status` | business data | stock accounting | contract performance | same |

No buyer id, name, address or contact data is stored (the hold has no buyer column). Logs may carry `reservation_id` and `campaign_id` only, never quantities tied to a person or any buyer field. Nothing new goes to analytics.

## Threat model

- **Assets:** flash-sale stock and price integrity, the `promotion.reserve` service credential, campaign counters.
- **Abuse cases:**
  - A browser user or seller holds all stock to block others (denial of sale), or releases someone else's hold: the RPCs are service-only; user principals (even carrying the scope string) are `PERMISSION_DENIED`, anonymous `UNAUTHENTICATED`, and the gateway does not route them.
  - A user commits a hold without paying to inflate `stock_sold`, or releases a committed hold to give stock back: same service-only rule; only team-order commits and releases.
  - Overselling by concurrent reserves: campaign row lock inside the transaction; tested with concurrent goroutines and in e2e.
  - Replay: a retried reserve or commit is idempotent on `reservation_id`; a second commit never double counts; a commit after release or expiry is refused (`committed=false`); a repeated release never decrements `stock_sold` twice.
  - Price spoofing: reserve refuses when `expected_sale_price` differs from the stored campaign price; the price is never taken from the request.
  - Guessing reservation ids: the ids only act inside the service-only RPCs, so guessing is not reachable by users.
  - Residual: a hold that team-order never commits or releases (crash between reserve and compensation) frees itself 15 minutes after reserve, so stock is not stranded; a committed hold whose order is never released stays sold until team-order releases it (cancel path).
- **Authz rule:** all three RPCs require a principal of type SERVICE holding scope `promotion.reserve`. `GetFlashSaleStock` and `GetActiveFlashSale` stay public.
