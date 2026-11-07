## ADDED Requirements

### Requirement: robots.txt guards private prefixes and points to the sitemap

`GET /robots.txt` SHALL answer 200 `text/plain` with `User-agent: *`, `Allow: /`, a `Disallow` line for each of
`/account`, `/cart`, `/checkout`, `/seller`, `/admin`, `/login`, `/register`, `/chat`, `/notifications`,
`/favorites`, `/dev` and `/api/`, and `Sitemap: <NEXT_PUBLIC_SITE_URL>/sitemap.xml`. It SHALL make no gateway call.

#### Scenario: SEO-21 robots.txt disallows private prefixes and lists the sitemap

- **WHEN** a crawler fetches `/robots.txt`
- **THEN** the response is 200, contains a `Disallow` for each of the twelve prefixes, and ends with `Sitemap: <NEXT_PUBLIC_SITE_URL>/sitemap.xml`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-21 robots.txt disallows private prefixes and lists the sitemap`

#### Scenario: SEO-22 robots.txt does not depend on the gateway

- **WHEN** the `robots` function is called with every gateway client mocked to throw
- **THEN** no gateway call is made and the rules and sitemap line are returned unchanged
- **VERIFIED BY**: `team-frontend/src/app/robots.test.ts` `robots makes no gateway call`

#### Scenario: SEO-30 An anonymous crawler is told to stay out of seller and admin prefixes

- **WHEN** an anonymous crawler reads `/robots.txt`
- **THEN** the lines `Disallow: /seller` and `Disallow: /admin` are present, although those routes are served by other apps
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-30 An anonymous crawler is told to stay out of seller and admin prefixes`

### Requirement: sitemap.xml lists public pages only, newest listings first

`GET /sitemap.xml` SHALL answer 200 `application/xml` listing the home URL, every category URL
(`/search?category=<id>`), the `published` listings as `/listing/<id>` ordered newest first up to 5,000, and the shop
`/shop/<sellerId>` of each listed listing once, with `sellerId` taken from `SearchHit.seller_id` and no per-hit `getListing` call.
The listings SHALL be read with `page_size` 50, the server maximum (`team-search/internal/rpc/search.go:24`), so 5,000
listings take 100 requests. Every `<loc>` SHALL be absolute on `NEXT_PUBLIC_SITE_URL`. The sitemap
SHALL NOT list a listing whose status is not `published`, and SHALL NOT list any private prefix. It SHALL render in
A cold render of 5,000 listings SHALL be measured and its duration reported; the 2 s figure is a target, not a pass condition. A hit with an empty `seller_id` SHALL contribute its listing URL and no shop URL. When the gateway fails, it SHALL answer 200 with the entries it already has, home at minimum.

#### Scenario: SEO-23 The sitemap lists home, categories, listings and shops

- **WHEN** a crawler fetches `/sitemap.xml` on a stack with 3 categories, 4 published listings of 2 sellers
- **THEN** the document holds the home URL, 3 category URLs, 4 `/listing/<id>` URLs newest first, and 2 `/shop/<sellerId>` URLs, all absolute
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-23 The sitemap lists home, categories, listings and shops`

#### Scenario: SEO-24 The sitemap omits unpublished listings

- **WHEN** a seller's listing is held or banned and a crawler fetches `/sitemap.xml`
- **THEN** that listing's URL is absent
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-24 The sitemap omits unpublished listings`

#### Scenario: SEO-25 The sitemap keeps the 5000 newest listings

- **WHEN** the sitemap function reads a search index that holds 5,200 published listings
- **THEN** exactly 5,000 `/listing/<id>` entries are returned in newest-first order and the 200 oldest are absent
- **VERIFIED BY**: `team-frontend/src/app/sitemap.test.ts` `caps at 5000 newest listings`

#### Scenario: SEO-36 A cold sitemap with 5000 listings is measured

- **WHEN** a crawler fetches `/sitemap.xml` on a cold cache after 5,000 published listings are seeded
- **THEN** the response is 200 with 5,000 listing URLs, produced by 100 `SearchListings` requests of `page_size` 50 with no `getListing` call, and the cold-render time is recorded in the run output
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-36 A cold sitemap with 5000 listings is measured`

#### Scenario: SEO-45 A hit without seller_id adds no shop URL

- **WHEN** the sitemap function reads a hit with an empty `seller_id` and a hit with `seller_id` `s1`
- **THEN** both listing URLs are present and exactly one shop URL, `/shop/s1`, is present
- **VERIFIED BY**: `team-frontend/src/app/sitemap.test.ts` `empty seller_id adds no shop url`

#### Scenario: SEO-26 A gateway failure leaves home in the sitemap

- **WHEN** the search call fails while a crawler fetches `/sitemap.xml`
- **THEN** the response is 200 and contains at least the home URL
- **VERIFIED BY**: `team-frontend/src/app/sitemap.test.ts` `gateway failure returns home`

### Requirement: No demo brand strings and a favicon

No `.ts` or `.tsx` source of a public page (non-test files under `team-frontend/src`) and no public UI string rendered by
`team-frontend` SHALL contain "Marketplace Showcase", "Polyrepo" or "BATDONGSAN SINH VIÊN". The site SHALL serve a favicon
declared by `<link rel="icon">` that answers 200 with an image content type.

#### Scenario: SEO-27 No public page shows a demo brand string

- **WHEN** a visitor opens `/`, `/search` and the student rental page and reads the page text and `<title>`
- **THEN** none contains "Marketplace Showcase", "Polyrepo" or "BATDONGSAN SINH VIÊN"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-27 No public page shows a demo brand string`

#### Scenario: SEO-28 A source scan rejects a demo brand string

- **WHEN** `brandStrings.test.ts` scans every non-test `.ts` and `.tsx` file under `team-frontend/src` and one contains a demo brand string
- **THEN** the test fails, naming the file
- **VERIFIED BY**: `team-frontend/src/app/brandStrings.test.ts` `no demo brand string in source`

#### Scenario: SEO-29 The favicon is served

- **WHEN** a browser follows the `<link rel="icon">` href found on `/`
- **THEN** the response is 200 with a content type starting `image/`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seo.feature` `SEO-29 The favicon is served`
