# seo-metadata Specification

## Purpose
TBD - created by archiving change frontend-seo. Update Purpose after archive.

## Requirements

### Requirement: Root metadata carries the Agora brand

The root layout SHALL export typed `Metadata` with `metadataBase` set to the origin in `NEXT_PUBLIC_SITE_URL` (default
`http://localhost:3000`, trailing slash removed), the title template `%s | Agora`, the default title
`Agora | Sàn thương mại điện tử` and a Vietnamese default description. The origin SHALL NOT be derived from request
headers. The root SHALL NOT set `openGraph`, `alternates` or `robots`. A page title SHALL end with exactly one
` | Agora`.

#### Scenario: SEO-1 A page title ends with one Agora suffix

- **WHEN** an anonymous visitor fetches `/assistant`, whose page title was once hand-written with the suffix `| Sàn Thương Mại Điện Tử`
- **THEN** the response `<title>` is `Trợ Lý Mua Sắm AI Thông Minh | Agora`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-1 A page title ends with one Agora suffix`

#### Scenario: SEO-2 The home page uses the default title and description

- **WHEN** an anonymous visitor fetches `/`
- **THEN** the response `<title>` is `Agora | Sàn thương mại điện tử`, `<meta name="description">` is non-empty Vietnamese text, and `link rel=canonical` is `<NEXT_PUBLIC_SITE_URL>` with no trailing slash
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-2 The home page uses the default title and description`

#### Scenario: SEO-3 A forged Host header does not change the canonical origin

- **WHEN** a visitor fetches `/` with the request header `Host: evil.example`
- **THEN** the canonical and `og:url` values start with `NEXT_PUBLIC_SITE_URL` and none contains `evil.example`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-3 A forged Host header does not change the canonical origin`

### Requirement: A listing page unfurls as a product

`/listing/<id>` SHALL render `<title>` as `<listing title> | Agora`, a Vietnamese meta description (the listing
description collapsed to one line and cut to at most 160 characters, or `Mua <title> giá <price> ₫ trên Agora.` when the
description is empty), `link rel=canonical` as the absolute URL `<NEXT_PUBLIC_SITE_URL>/listing/<id>`, and
`og:title`, `og:description`, `og:url`, `og:site_name=Agora`, `og:type=product` and `og:image` (the listing's first
image, absolute). When the listing has no image, `og:image` SHALL be omitted. The page SHALL make one `getListing`
call per request for layout, page and metadata together: `loadListing` is the same request-cached call for all three, so the
page status and the metadata see the same visibility result. A listing that does not exist SHALL answer 404 with no `og:*`
tag and no canonical. When the listing read fails with an error other than not found, `generateMetadata` SHALL return the
root default metadata instead of throwing.

#### Scenario: SEO-4 A listing page has title, description, canonical and Open Graph tags

- **WHEN** an anonymous crawler fetches `/listing/<id>` of a published listing with an image
- **THEN** `<title>` is `<listing title> | Agora`, `og:type` is `product`, `og:site_name` is `Agora`, `og:url` equals the canonical `<NEXT_PUBLIC_SITE_URL>/listing/<id>`, and `og:image` is an absolute URL of the first image
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-4 A listing page has title, description, canonical and Open Graph tags`

#### Scenario: SEO-5 A missing listing answers 404 without Open Graph tags

- **WHEN** a crawler fetches `/listing/does-not-exist`
- **THEN** the response status is 404 and the HTML contains no `og:*` meta tag and no `link rel=canonical`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-5 A missing listing answers 404 without Open Graph tags`

#### Scenario: SEO-44 A held, banned or draft listing answers 404 without metadata

- **WHEN** an anonymous crawler fetches `/listing/<id>` of a listing whose status is held, banned or draft
- **THEN** the response status is 404, the HTML contains no `og:*` meta tag and no `link rel=canonical`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-44 A held, banned or draft listing answers 404 without metadata`

#### Scenario: SEO-6 A long description is cut at 160 characters

- **WHEN** a crawler fetches the page of a listing whose description is 400 characters with line breaks
- **THEN** `<meta name="description">` and `og:description` hold at most 160 characters and no line break
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-6 A long description is cut at 160 characters`

#### Scenario: SEO-7 A listing without an image omits og:image

- **WHEN** a crawler fetches the page of a published listing that has no image
- **THEN** the HTML contains no `og:image` tag and the other `og:*` tags are present
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-7 A listing without an image omits og:image`

#### Scenario: SEO-31 A listing read outage does not break the metadata build

- **WHEN** `getListing` rejects with an Unavailable error while `generateMetadata` runs for `/listing/<id>`
- **THEN** `generateMetadata` resolves to the root default title and description and does not throw
- **VERIFIED BY**: `team-frontend/src/app/(shop)/listing/[id]/metadata.test.ts` `listing outage returns default metadata`

#### Scenario: SEO-8 A listing metadata build makes no extra gateway call

- **WHEN** the listing layout, page and `generateMetadata` run for one request
- **THEN** `getListing` is called exactly once
- **VERIFIED BY**: `team-frontend/src/app/(shop)/listing/[id]/metadata.test.ts` `getListing is called once per request`

### Requirement: A shop page unfurls with the shop identity

`/shop/<id>` SHALL render `<title>` as `<shop name> | Agora` where the name is the storefront `displayName` or, when
unset, the existing shop label fallback; a Vietnamese description (the tagline, else `Cửa hàng <name> trên Agora`);
`link rel=canonical` `<NEXT_PUBLIC_SITE_URL>/shop/<id>`; and `og:title`, `og:description`, `og:url`,
`og:site_name=Agora`, `og:type=website` and `og:image` set to the storefront banner when one exists. The page SHALL
make one `getStorefront` call per request for page and metadata together. When the storefront read fails, or the seller id
has no storefront, the page SHALL answer 200 with the fallback shop name, keep the canonical and omit `og:image`; no 404 is
added for an unknown shop id.

#### Scenario: SEO-9 A shop page uses the shop name and banner

- **WHEN** an anonymous crawler fetches `/shop/<sellerId>` for a seller with display name "Cửa hàng Mai" and a banner
- **THEN** `<title>` is `Cửa hàng Mai | Agora`, `og:image` is the absolute banner URL, and the canonical is `<NEXT_PUBLIC_SITE_URL>/shop/<sellerId>`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-9 A shop page uses the shop name and banner`

#### Scenario: SEO-10 A shop whose storefront read fails still renders

- **WHEN** the gateway answers an error to `getStorefront` for `/shop/<id>`
- **THEN** `generateMetadata` resolves to the fallback shop name followed by ` | Agora` with no `og:image` and does not throw. The 200 status is asserted in e2e where practical.
- **VERIFIED BY**: `team-frontend/src/app/(shop)/shop/[id]/metadata.test.ts` `storefront failure falls back`

#### Scenario: SEO-32 An unknown shop id renders with the fallback name

- **WHEN** a crawler fetches `/shop/unknown-seller` and `getStorefront` returns null
- **THEN** `generateMetadata` resolves to the fallback shop label followed by ` | Agora` with no `og:image` and does not throw. The 200 status is asserted in e2e where practical.
- **VERIFIED BY**: `team-frontend/src/app/(shop)/shop/[id]/metadata.test.ts` `unknown shop id falls back`

#### Scenario: SEO-11 A shop metadata build makes no extra gateway call

- **WHEN** `generateMetadata` and the shop page run for one request
- **THEN** `getStorefront` is called exactly once
- **VERIFIED BY**: `team-frontend/src/app/(shop)/shop/[id]/metadata.test.ts` `getStorefront is called once per request`

### Requirement: Category pages are indexable and free-text searches are not

`/search?category=<id>` for an existing category SHALL render `<title>` as `<category name> | Agora`, a Vietnamese
description, `link rel=canonical` `<NEXT_PUBLIC_SITE_URL>/search?category=<id>` (every other query parameter dropped),
the `og:*` set with `og:type=website`, and no `noindex`. `/search` without `q` and without a category SHALL be
indexable with title `Tất cả sản phẩm | Agora`, description `Tìm và mua sản phẩm từ nhiều người bán trên Agora.` and canonical
`<NEXT_PUBLIC_SITE_URL>/search`. `q` SHALL be trimmed; a whitespace-only `q` counts as empty. A request with a non-empty
trimmed `q` SHALL carry `robots: noindex,follow`, no canonical, and a title that does not contain the query text; when `q`
and `category` are both present, `q` wins and the page is noindex. A `category` value that
matches no category SHALL use the plain-search metadata. The page SHALL make one `listCategories` call per request for
page and metadata together.

#### Scenario: SEO-12 A category page is indexable with a clean canonical

- **WHEN** a crawler fetches `/search?category=<id>&sort=price_asc&page=2`
- **THEN** `<title>` is `<category name> | Agora`, the canonical is `<NEXT_PUBLIC_SITE_URL>/search?category=<id>`, and no `noindex` robots tag is present
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-12 A category page is indexable with a clean canonical`

#### Scenario: SEO-13 A free-text search is noindex and follow

- **WHEN** a crawler fetches `/search?q=<script>x</script>`
- **THEN** the robots meta is `noindex, follow`, there is no `link rel=canonical`, and the query text appears in no `<title>` or `og:*` tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-13 A free-text search is noindex and follow`

#### Scenario: SEO-33 A whitespace-only q counts as empty

- **WHEN** a crawler fetches `/search?q=%20%20%20`
- **THEN** `generateMetadata` resolves to title `Tất cả sản phẩm`, canonical `<NEXT_PUBLIC_SITE_URL>/search` and no `noindex` robots, and does not throw. The 200 status is asserted in e2e where practical.
- **VERIFIED BY**: `team-frontend/src/app/(shop)/search/metadata.test.ts` `whitespace q is empty`

#### Scenario: SEO-34 q wins over category

- **WHEN** a crawler fetches `/search?category=<id>&q=ao`
- **THEN** `generateMetadata` resolves to robots `noindex, follow`, no canonical and a title that is not the category name, and does not throw. The 200 status is asserted in e2e where practical.
- **VERIFIED BY**: `team-frontend/src/app/(shop)/search/metadata.test.ts` `q wins over category`

#### Scenario: SEO-35 Plain search is indexable with its own title and description

- **WHEN** a crawler fetches `/search`
- **THEN** `<title>` is `Tất cả sản phẩm | Agora` and `<meta name="description">` is `Tìm và mua sản phẩm từ nhiều người bán trên Agora.`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-35 Plain search is indexable with its own title and description`

#### Scenario: SEO-14 An unknown category falls back to plain search metadata

- **WHEN** a crawler fetches `/search?category=no-such-category`
- **THEN** `<title>` is `Tất cả sản phẩm | Agora` and the canonical is `<NEXT_PUBLIC_SITE_URL>/search`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/search/metadata.test.ts` `unknown category uses plain search metadata`

#### Scenario: SEO-15 A categories outage keeps the page rendering

- **WHEN** `listCategories` fails during `generateMetadata` for `/search?category=<id>`
- **THEN** `generateMetadata` resolves to title `Tất cả sản phẩm` and does not throw. The 200 status is asserted in e2e where practical.
- **VERIFIED BY**: `team-frontend/src/app/(shop)/search/metadata.test.ts` `categories failure falls back`

### Requirement: Private pages are noindex and nofollow

Every page under the `account`, `cart`, `checkout`, `chat`, `notifications`, `favorites`, `dev`, `login` and `register`
prefixes of `team-frontend` SHALL carry `robots: noindex,nofollow`, SHALL NOT carry any `og:*` tag, and SHALL NOT carry
a canonical link, whether or not the visitor is logged in. `login` SHALL keep `referrer: no-referrer`.

#### Scenario: SEO-16 An anonymous crawler gets noindex on the private pages that render

- **WHEN** an anonymous crawler fetches `/login` and `/register`, the private URLs that render HTML without a session
- **THEN** each response contains `<meta name="robots" content="noindex, nofollow">` and no `og:*` tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-16 An anonymous crawler gets noindex on the private pages that render`

#### Scenario: SEO-17 A logged-in buyer gets noindex on the guest-redirected pages

- **WHEN** a logged-in buyer fetches `/cart`, `/favorites`, `/notifications`, `/chat`, `/account/orders` and `/account/addresses`, which redirect a guest to `/login`
- **THEN** each response contains `<meta name="robots" content="noindex, nofollow">` and no `og:*` tag
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-17 A logged-in buyer gets noindex on the guest-redirected pages`

#### Scenario: SEO-18 The checkout group layout exports noindex

- **WHEN** the `(checkout)` layout is scanned by `seoPrivateRoutes.test.ts`
- **THEN** its `metadata` export contains `robots` with `index: false` and `follow: false`
- **VERIFIED BY**: `team-frontend/src/app/seoPrivateRoutes.test.ts` `every private segment layout exports noindex`

#### Scenario: SEO-19 The login page keeps its no-referrer declaration beside the robots layout

- **WHEN** `referrerMetadata.test.ts` and `seoPrivateRoutes.test.ts` scan `login/page.tsx` and `login/layout.tsx`
- **THEN** the page's `metadata` block still declares `referrer: "no-referrer"` and the layout's `metadata` block declares `robots` with `index: false`, so Next merges both keys
- **VERIFIED BY**: `team-frontend/src/app/referrerMetadata.test.ts` `login sets no-referrer (Referrer hygiene: login declares no-referrer)`
- **VERIFIED BY**: `team-frontend/src/app/seoPrivateRoutes.test.ts` `every private segment layout exports noindex`

#### Scenario: SEO-20 A new private segment without the policy fails the build

- **WHEN** a segment layout under one of the private prefixes lacks `robots` with `index: false`
- **THEN** `seoPrivateRoutes.test.ts` fails, naming the segment
- **VERIFIED BY**: `team-frontend/src/app/seoPrivateRoutes.test.ts` `scanner rejects a private layout without robots`
