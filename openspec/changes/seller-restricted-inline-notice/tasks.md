## 1. Session package

- [x] 1.1 Failing test: `decodePrincipal` returns `status: "restricted"` for a token with that claim and no status otherwise; implement; changeset; `npm run vendor` and `vendor:check`

## 2. Seller app

- [x] 2.1 Failing tests: the layout renders the buyer and the restricted variants in place without calling `redirect`, renders no children, and the buyer variant has no "Đăng ký bán hàng" link
- [x] 2.2 `SellerAccessDenied`, `getPrincipal` keeps `status`, layout renders inline; delete `src/app/(auth)/forbidden/`; verify the tests pass

## 3. e2e and FEATURES

- [x] 3.1 `still_on` and `login_return` use the seller host for seller paths; restricted scenario asserts the new title; new buyer scenario
- [x] 3.2 Move the restricted entry to `team-frontend-seller/FEATURES.yaml`, update the related entries; `make -C platform-e2e features-check`

## 4. Docs

- [x] 4.1 ADR-0006, `fe-platform/docs/gaps.md`, seller README

## 5. Verification

- [x] 5.1 `npm run check` in fe-platform and team-frontend-seller, `npm run build` in the seller app
- [x] 5.2 Live e2e of the restricted, buyer and login-return scenarios on a minimal stack
- [x] 5.3 `openspec validate seller-restricted-inline-notice --strict`
