## Account age source

`AuthService.GetMyAccount` (principal-scoped, routed for any signed-in user) returns `{user_id, created_at}` (clarification C1).
`accountAgeMs = now - created_at`; the panel shows when it is under 14 days. When `GetMyAccount` fails or returns no
`created_at`, the panel is hidden entirely (fail closed: an old account must not see a welcome banner). The returned
`user_id` is not rendered or logged.

## Placement

`WelcomePanel` is an async server component. On `/` it sits above `Hero` in its own `Suspense fallback={null}` next to
`LoyaltyBlock`. On `/account/*` it renders in `AccountShell` above the content. `/account` itself has no route today, so
"on /account" is satisfied by the shell. Signed-out visitors render nothing and trigger no gateway call.

## Reads and degrade

The panel runs in parallel: `getMyAccount`, `listMyVouchers`, `listFollowedSellers`, `listAddresses`,
`listBuyerOrders`. The existing wrappers for the last three already return `[]` on failure, so a failed read shows the
step as not done (matching the locked story). The first-order step ticks when the buyer has at least one order whose status is not
`ORDER_STATUS_CANCELLED` (clarification C6), filtered client-side from `ListBuyerOrders`.
`listMyVouchers` returns a tagged result `ok | failed`: `ok` with no `NEW_USER` voucher shows "Voucher đang được chuẩn bị" only while the account is under 1 hour old (voucher issue is async and the verdict fallback is 30 min); later it shows no voucher line. A `NEW_USER` voucher that is expired or used shows no code and no pending text;
`failed` omits the voucher line so a read outage is not reported as "being prepared".

## Dismissal

A server action `dismissWelcome(part: "banner" | "checklist")` sets the cookie `agora_welcome_dismissed` or
`agora_checklist_dismissed`, then `revalidatePath` of the current path. The server render reads the cookie, so there is
no flash of dismissed content. The checklist also hides without a cookie once all three steps are done.

## Voucher selection

Banner voucher = first `ListMyVouchers` entry with `kind = NEW_USER` whose `ends_at` is in the future. Amounts use
`formatPrice` for đ (the voucher is a discount, unlike xu); expiry uses the existing date formatter.

## Checkout picker

The checkout page fetches `listMyVouchers()` once in the payment step and passes it to `VoucherModal`, which merges it
before the platform vouchers in the platform slot, deduplicated by code. A failed read leaves the platform list as is.

## Timeouts

Every panel read and the checkout picker read use a 2 s timeout. A timeout is treated as a failure of that read only:
the panel part is degraded, the picker keeps the platform list, and no error banner is shown.
