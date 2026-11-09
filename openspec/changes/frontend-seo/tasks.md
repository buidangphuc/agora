## 1. Code track (team-frontend)

- [x] 1.1 Tests first: `src/lib/seo.test.ts` (siteUrl default and trailing slash, absoluteUrl, description cut at 160, no Host use) — SEO-3, SEO-6; verify red
- [x] 1.2 Tests first (sitemap cases include an empty `seller_id`): `src/app/robots.test.ts`, `src/app/sitemap.test.ts` (cap 5000 newest, failure returns home), `src/app/seoPrivateRoutes.test.ts` (layout scanner incl. rejecting case, and the case `login layout noindex beside page referrer`), `src/app/brandStrings.test.ts` — SEO-20, SEO-22, SEO-25, SEO-26, SEO-28, SEO-45; verify red
- [x] 1.3 Tests first: `metadata.test.ts` beside `listing/[id]`, `shop/[id]`, `search` (call counts, fallbacks, unknown category) — SEO-8, SEO-10, SEO-11, SEO-14, SEO-15, SEO-31, SEO-32, SEO-33, SEO-34; verify red
- [x] 1.4 `src/lib/seo.ts` (`siteUrl`, `absoluteUrl`, `pageMetadata`) and root `layout.tsx` typed metadata (metadataBase, template, default title and description) — SEO-1, SEO-2, SEO-3
- [x] 1.5 `generateMetadata` for `/listing/[id]` (product, image omission, 404 path untouched) — SEO-4, SEO-5, SEO-6, SEO-7, SEO-8, SEO-31, SEO-44
- [x] 1.6 `shop/[id]/data.ts` `loadStorefront = requestCache(getStorefront)`, page and `generateMetadata` use it — SEO-9, SEO-10, SEO-11, SEO-32
- [x] 1.7 `generateMetadata` for `/search` (category title, plain-search title and description, canonical, trimmed `q` noindex,follow, `q` over category); keep the edit to the metadata export only, the page body is shared with search-bestseller-rating and search-attribution — SEO-12, SEO-13, SEO-14, SEO-15, SEO-33, SEO-34, SEO-35
- [x] 1.8 Home `generateMetadata` (canonical `<site>` without trailing slash, `og:*`) — SEO-2
- [x] 1.9 Private segment layouts (`account`, `cart`, `chat`, `notifications`, `favorites`, `dev`, `login`, `register`) and `(checkout)/layout.tsx` robots export; run `referrerMetadata.test.ts` — SEO-16, SEO-17, SEO-18, SEO-19, SEO-20
- [x] 1.10 Remove hand-written title suffixes on the ten pages listed in the proposal — SEO-1
- [x] 1.11 `src/app/robots.ts`, `src/app/sitemap.ts` with `src/lib/gateway/sitemap.ts` reader (`page_size` 50, `SearchHit.seller_id`, no `getListing`), `revalidate = 3600`; depends on the regenerated stubs from discovery-seo-contract — SEO-21, SEO-22, SEO-30, SEO-23, SEO-24, SEO-25, SEO-26, SEO-36, SEO-45
- [x] 1.12 Brand strings in `StudentRentalView.tsx`, `ConsumerShell.tsx`, checkout header link; `src/app/icon.svg` — SEO-27, SEO-28, SEO-29
- [x] 1.13 Document `NEXT_PUBLIC_SITE_URL` in the env docs and the compose frontend service — SEO-3

## 2. Code track (team-frontend-seller, team-frontend-admin)

- [x] 2.1 Tests first: `team-frontend-seller/src/app/titleSuffix.test.ts` and `team-frontend-admin/src/app/titleSuffix.test.ts` (scan page titles for the three suffixes; extend `team-frontend-admin/src/app/referrerMetadata.test.ts` to the root `robots`) — SEO-41, SEO-43, SEO-42; verify red
- [x] 2.2 Seller root layout: `robots` noindex,nofollow, title `{ default: "Kênh Người Bán | Agora", template: "%s | Agora" }`; strip `| Kênh người bán` from the 9 seller pages that carry it — SEO-37, SEO-38, SEO-41
- [x] 2.3 Admin root layout: keep `referrer`, keep `robots`, title `{ default: "Quản trị sàn | Agora", template: "%s | Agora" }`; strip `| Sàn Thương Mại Điện Tử` from the 11 admin pages and update tests that assert the old titles — SEO-39, SEO-40, SEO-43, SEO-42

## 3. e2e track (platform-e2e, team-frontend FEATURES.yaml)

- [x] 3.1 New `platform-e2e/tests/e2e/features/frontend/seo.feature` with scenarios SEO-1 to SEO-7, SEO-9, SEO-12, SEO-13, SEO-44, SEO-16, SEO-17, SEO-21, SEO-30, SEO-23, SEO-24, SEO-27, SEO-29, SEO-35, SEO-36, SEO-37, SEO-38, SEO-39, SEO-40 and steps (on the seller :3001 and admin :3002 apps too; HTTP fetch of raw HTML, tag-seeded listings and sellers, a listing with no image, a held listing)
- [x] 3.2 SEO-3 step sends the forged `Host` header; SEO-6 seeds a 400-character multi-line description
- [x] 3.3 `team-frontend/FEATURES.yaml` entries `seo.metadata`, `seo.private-noindex`, `seo.crawl-files`, `seo.private-apps` pointing `covered_by` at the scenarios above (done by the e2e-author, serialised with the other discovery-seo slices)
- [x] 3.4 Rebuild `team-frontend` into the local stack and run `seo.feature`; time a cold `/sitemap.xml` with 5,000 seeded listings and record the duration and the request count (100) in the run output; the 2 s figure is a target, not a gate; if seeding 5,000 listings is not feasible locally, record the reason in the run output instead of failing — SEO-36
