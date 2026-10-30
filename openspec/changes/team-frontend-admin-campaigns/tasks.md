## 1. Contract

- [x] 1.1 Confirm RPCs, scopes and audit behaviour in platform-core proto, team-gateway routing and team-promotion handlers

## 2. Gateway wrapper

- [x] 2.1 `campaigns.ts` with `flashSale` and `subscription` clients in `makeClients`; verify fake-transport tests for mapping and every error code

## 3. Screens

- [x] 3.1 Form parsing and Server Action with re-validation and error mapping; verify unit tests
- [x] 3.2 `/admin/campaigns`, `/admin/campaigns/new` with review dialog, `/admin/plans`, nav entries; verify component and page tests

## 4. Docs

- [x] 4.1 Gaps G9-G11 in `fe-platform/docs/gaps.md` and the admin README routes and known gaps

## 5. Verification

- [x] 5.1 `npm run check` and `npm run build` in `team-frontend-admin`
- [x] 5.2 Browser check at 375 px: form, review dialog, audit-down error, no horizontal scroll
- [x] 5.3 `openspec validate team-frontend-admin-campaigns --strict`
