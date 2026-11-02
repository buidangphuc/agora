## 1. Gateway

- [x] 1.1 `vouchers.ts` (list with empty seller id, get by code, create with scope PLATFORM, status by code) and the `voucher` client in `makeClients`; verify the fake-transport tests in `vouchers.test.ts`

## 2. Feature

- [x] 2.1 Shared validation and formatting with tests; verify `voucherForm.test.ts`
- [x] 2.2 `createPlatformVoucherAction` with Vietnamese messages and the audit-down case; verify `actions.test.ts`
- [x] 2.3 List, details and create form with the two-step confirm; verify `CreateVoucherForm.test.tsx`

## 3. Routes and nav

- [x] 3.1 `/admin/vouchers`, `/admin/vouchers/new`, `/admin/vouchers/[code]` with gates and metadata; nav entry visible; verify the page tests, `referrerMetadata.test.ts` and `AdminShell.test.tsx`

## 4. Docs

- [x] 4.1 README contract and known gaps, gaps G9 to G11 in `fe-platform/docs/gaps.md`

## 5. Verification

- [x] 5.1 `npm run check` and `npm run build` in `team-frontend-admin`
- [x] 5.2 `openspec validate team-frontend-admin-vouchers --strict`
