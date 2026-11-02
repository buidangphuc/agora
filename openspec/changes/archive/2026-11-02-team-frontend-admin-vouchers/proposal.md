## Why

Plan P10 step E1c: the admin app exists to manage platform vouchers and other marketplace-wide actions, and it had no voucher screens (the nav entry was hidden). team-promotion already exposes `VoucherService.ListVouchers`, `GetVoucher` and `CreateVoucher` through the gateway, so admins can list, create and inspect platform vouchers without any backend change.

## What Changes

- `team-frontend-admin/src/lib/gateway/vouchers.ts`: `listPlatformVouchers(cursor)` (empty `seller_id`, page size 20), `getVoucher(code)`, `createPlatformVoucher(input)` (scope `PLATFORM`); results carry a status by Connect code and never the backend message. `makeClients` gains the `voucher` client.
- `src/features/vouchers/`: form validation shared by the client and the Server Action, `createPlatformVoucherAction` with Vietnamese messages per status, list view, details view, create form with a review dialog and an acknowledgement checkbox before the create call, success view built from the response.
- Routes `/admin/vouchers`, `/admin/vouchers/new`, `/admin/vouchers/[code]` with the same guest, scope and gateway-denied gates as the other admin pages and `referrer: no-referrer`.
- Admin nav shows "Voucher sàn"; campaigns stay hidden.
- Docs: `team-frontend-admin/README.md` (contract, known gaps), `fe-platform/docs/gaps.md` rows G9 (no edit, stop or delete RPC), G10 (duplicate code is not `ALREADY_EXISTS`), G11 (audit-down and promotion-down look the same).

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-admin/src/{lib/gateway,features/vouchers,app/admin/vouchers,components/shell}`, `team-frontend-admin/README.md`, `fe-platform/docs/gaps.md`.
- Visible changes: a "Voucher sàn" nav entry and three new admin screens. No change to the buyer or seller apps, the gateway or team-promotion.

## Non-goals

Campaign screens (E1d), editing, stopping or deleting vouchers (no RPC), shop vouchers, backend changes, moving the gateway client to `@fe-platform/api-client`, the live run on a minimal stack and the buyer checkout redemption check.

## Non-functional requirements

`npm run check` and `npm run build` green for `team-frontend-admin`; no backend message in any URL or rendered error; no new dependency; touch targets of the new controls at least 44px through the platform components.
