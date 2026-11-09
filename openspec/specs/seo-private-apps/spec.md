# seo-private-apps Specification

## Purpose
TBD - created by archiving change frontend-seo. Update Purpose after archive.

## Requirements

### Requirement: Seller and admin apps are noindex and use the Agora title template

The root layout of `team-frontend-seller` and of `team-frontend-admin` SHALL export `metadata` with
`robots: {index: false, follow: false}` and a title template `%s | Agora`. Their default titles SHALL be
`Kênh Người Bán | Agora` and `Quản trị sàn | Agora`. No page title in either app SHALL contain a hand-written suffix
(`| Kênh người bán`, `| Sàn Thương Mại Điện Tử`, `| Marketplace`), so every page title ends with exactly one ` | Agora`.
The admin root SHALL keep `referrer: "no-referrer"`.

#### Scenario: SEO-37 A seller page is noindex and titled with one Agora suffix

- **WHEN** a logged-in seller fetches `/seller/wallet` on `team-frontend-seller`
- **THEN** `<title>` is `Ví người bán | Agora` (source title `Ví người bán`, `team-frontend-seller/src/app/seller/wallet/page.tsx:28`) and the robots meta is `noindex, nofollow`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-37 A seller page is noindex and titled with one Agora suffix`

#### Scenario: SEO-38 The seller login page is noindex for an anonymous crawler

- **WHEN** an anonymous crawler fetches `/login` on `team-frontend-seller`
- **THEN** the robots meta is `noindex, nofollow` and `<title>` ends with exactly one ` | Agora`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-38 The seller login page is noindex for an anonymous crawler`

#### Scenario: SEO-39 An admin page is noindex, titled once and keeps no-referrer

- **WHEN** an admin with scope `admin` fetches `/admin/audit` on `team-frontend-admin`
- **THEN** `<title>` is `Nhật ký kiểm toán | Agora` (source title `Nhật ký kiểm toán`, `team-frontend-admin/src/app/admin/audit/page.tsx:15`), the robots meta is `noindex, nofollow`, and the referrer meta is `no-referrer`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-39 An admin page is noindex, titled once and keeps no-referrer`

#### Scenario: SEO-40 The admin login and forbidden pages deny indexing for an anonymous crawler

- **WHEN** an anonymous crawler fetches `/login` and `/forbidden` on `team-frontend-admin`
- **THEN** each response contains the robots meta `noindex, nofollow` and the referrer meta `no-referrer`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-40 The admin login and forbidden pages deny indexing for an anonymous crawler`

#### Scenario: SEO-41 A hand-written suffix in a seller page fails the build

- **WHEN** `team-frontend-seller/src/app/titleSuffix.test.ts` scans the `metadata` titles of every page under `team-frontend-seller/src/app` and one contains `| Kênh người bán` or `| Marketplace`
- **THEN** the test fails, naming the file
- **VERIFIED BY**: `team-frontend-seller/src/app/titleSuffix.test.ts` `no hand-written suffix in page titles`

#### Scenario: SEO-43 A hand-written suffix in an admin page fails the build

- **WHEN** `team-frontend-admin/src/app/titleSuffix.test.ts` scans the `metadata` titles of every page under `team-frontend-admin/src/app` and one contains `| Sàn Thương Mại Điện Tử` or `| Marketplace`
- **THEN** the test fails, naming the file
- **VERIFIED BY**: `team-frontend-admin/src/app/titleSuffix.test.ts` `no hand-written suffix in page titles`

#### Scenario: SEO-42 The admin root keeps its referrer and robots declarations

- **WHEN** `team-frontend-admin/src/app/referrerMetadata.test.ts` scans the root layout after the title template is added
- **THEN** the root `metadata` block still declares `referrer: "no-referrer"` and `robots` with `index: false`
- **VERIFIED BY**: `team-frontend-admin/src/app/referrerMetadata.test.ts` `admin root declares no-referrer`
