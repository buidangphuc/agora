## Why

A shared `/listing/<id>` link unfurls in chat apps and search engines with the title "Marketplace Showcase | Nền Tảng
Thương Mại Điện Tử Polyrepo": the root `team-frontend/src/app/layout.tsx` exports one untyped `metadata` object, no page
calls `generateMetadata`, `public/` is empty (no favicon, no `robots.txt`, no sitemap), and no private page except the
referrer-hygiene ones declares a robots policy. Brand strings left over from the demo ("BATDONGSAN SINH VIÊN",
"Polyrepo Showcase", "Marketplace Showcase Polyrepo") are visible in the shell and on the student rental page.

initiative: discovery-seo

Stories 7 and 8 and decision D7 of `plans/discovery-seo/00-spec.md`, clarifications C4, C8 and C9. `depends_on:
[discovery-seo-contract]`: the sitemap reads `SearchHit.seller_id`, added to the proto by that slice. No other new RPC
or event is consumed.

## What Changes

- Root layout of `team-frontend`: typed `Metadata` with `metadataBase` from `NEXT_PUBLIC_SITE_URL` (default `http://localhost:3000`),
  title `{ default: "Agora | Sàn thương mại điện tử", template: "%s | Agora" }`, and a Vietnamese default description.
  The root sets no `openGraph`, `alternates` or `robots`, so a 404 response inherits no OG tags and no canonical.
- New `src/lib/seo.ts`: `siteUrl()`, `absoluteUrl(path)`, and `pageMetadata({title, description, path, image, type, robots})`
  which builds title, description, canonical and the `openGraph` block (`og:title`, `og:description`, `og:url`,
  `og:site_name=Agora`, `og:image`, `og:type`). Public pages call it; nothing else sets OG tags.
- `generateMetadata` on `/` (static copy), `/listing/[id]` (`og:type=product`), `/shop/[id]` and `/search` (category
  title and description; `q=` gives `noindex,follow`). They reuse `loadListing`, `loadCategories`, and a new
  request-cached `loadStorefront` in `shop/[id]/data.ts`, so no extra gateway call is made.
- Private pages: a `robots: {index:false, follow:false}` export in the route layouts of `account`, `cart`, `chat`,
  `notifications`, `favorites`, `dev`, `login`, `register` (new segment layouts under `(shop)`) and in the `(checkout)`
  group layout. `login/page.tsx` keeps `referrer: "no-referrer"` and `referrerMetadata.test.ts` stays green.
- `team-frontend-seller` and `team-frontend-admin` root layouts: `robots: {index:false, follow:false}` and title template
  `%s | Agora` (default titles `Kênh Người Bán | Agora` and `Quản trị sàn | Agora`). Hand-written suffixes
  (`| Kênh người bán` on 9 seller pages, `| Sàn Thương Mại Điện Tử` on 11 admin pages, `| Marketplace` on the seller root
  title) are removed. The admin root keeps `referrer: no-referrer` and `referrerMetadata.test.ts` stays green. The seller
  and admin pages not set to noindex individually inherit it from the root.
- `src/app/robots.ts` and `src/app/sitemap.ts`; `src/app/icon.svg` (favicon).
- Title suffix cleanup: titles hand-written with a suffix (`assistant`, `notifications`, `favorites`, `account/referral`,
  `account/security`, `account/addresses`, `account/following`, `account/verification`, `vouchers`, `dev/ui`) drop it,
  so the template yields exactly one "| Agora".
- Brand strings: `StudentRentalView.tsx:109`, `ConsumerShell.tsx` (lines 235, 282, 294, 421, 433) and the checkout header
  link text "Marketplace" become "Agora" copy; no `.ts` or `.tsx` source of a public page (non-test files under
  `team-frontend/src`) contains "Marketplace Showcase", "Polyrepo" or "BATDONGSAN SINH VIÊN".
- `assistant` stays indexable.
- Config: `NEXT_PUBLIC_SITE_URL` documented in `team-frontend/.env.example` (or the README env table if no example file)
  and in the compose service definition for the frontend.

## Capabilities

### New Capabilities
- `seo-metadata`: titles, descriptions, canonical, Open Graph and per-page robots for public and private buyer pages.
- `seo-crawl-files`: `robots.txt`, `sitemap.xml`, favicon and brand hygiene.
- `seo-private-apps`: noindex and the `%s | Agora` title template in team-frontend-seller and team-frontend-admin.

### Modified Capabilities
none

## Impact

- `team-frontend/src/app/layout.tsx`, `src/app/robots.ts`, `src/app/sitemap.ts`, `src/app/icon.svg`, `src/lib/seo.ts`
  (+ test), `src/app/(shop)/(home)/page.tsx`, `listing/[id]/page.tsx`, `shop/[id]/{page.tsx,data.ts}`,
  `search/page.tsx`, new layouts for the private segments, `(checkout)/layout.tsx`, the suffix and brand files above,
  `team-frontend/FEATURES.yaml`.
- Hot files shared with other slices: `search/page.tsx` (search-bestseller-rating, search-attribution) and
  `FEATURES.yaml`; initiative-build serialises them.
- `platform-e2e/tests/e2e/features/frontend/seo.feature` (new) and its steps.
- `team-frontend-seller/src/app/layout.tsx` and the 9 seller pages with a suffix; `team-frontend-admin/src/app/layout.tsx`
  and the 11 admin pages with a suffix (tests asserting the old titles are updated).
- No proto, event or database change here (`SearchHit.seller_id` comes from `discovery-seo-contract`).

## Non-goals

JSON-LD structured data, hreflang, AMP, an `og:image` generator for listings without an image, a shop logo (the
storefront has only `banner_url`).

## Non-functional requirements

- `generateMetadata` makes no gateway call beyond the page's own cached loader: one `getListing` per PDP request, one
  `getStorefront` per shop request, one `listCategories` per search request (asserted by call counts in unit tests).
- `GET /sitemap.xml`: a cold render with 5,000 listings is measured and its duration reported (100 sequential
  `SearchListings` calls of `page_size` 50, the server maximum, no per-hit lookup; revalidated hourly). The 2 s figure is a
  target, not a pass condition.
- `GET /robots.txt` answers in under 200 ms and makes no gateway call.
- Availability: if the gateway is down, `generateMetadata` falls back to the default title and description and the page
  still renders; `sitemap.xml` lists home only and answers 200.
- Limit: at most 5,000 listing URLs, newest first (below the 50,000 URL / 50 MB sitemap protocol limit).

## Data classification

| Field | Purpose | Consent basis | Retention |
|---|---|---|---|
| Listing title, description, first image, price | Page title, description, `og:*` for a public listing | Public content the seller published | Not stored; rendered per request |
| Shop display name, tagline, banner | Page title, description, `og:*` for a public shop | Public content the seller published | Not stored |
| Category name | Page title of a category page | Public catalogue data | Not stored |
| `sellerId` in `/shop/<sellerId>` URLs (sitemap, canonical, `og:url`) | Locate a shop page | Public seller identifier, already public on the shop page and in listing data | Not stored; sitemap cached up to 1 h |

No personal data is added: metadata is built only from fields already visible to an anonymous visitor, private pages
emit no OG tags, and the sitemap lists only `published` listings and the shops that own them. Search query text
(`q=`) is never put in a title, description or canonical.

## Threat model

- Assets: unpublished listings and private account pages must not be discoverable; the site origin used in canonical
  and sitemap URLs.
- Abuse cases:
  1. A draft, held, banned or rejected listing leaks through the sitemap or OG tags. The sitemap reads only
     `status=published` search hits, and `loadListing` returns null for a listing the caller may not see, which yields
     404 with no OG tags.
  2. A reflected `q=` value injected into a title or canonical. `q` is never used in metadata; free-text search is
     `noindex,follow` and its canonical is dropped.
  3. A poisoned `Host` header changes canonical URLs. The origin comes only from `NEXT_PUBLIC_SITE_URL`, never from
     request headers.
  4. A listing description with markup. Next escapes metadata values; the description is truncated to 160 characters
     and stripped of newlines.
  5. Private pages indexed through a link from a public page. Their layouts emit `noindex,nofollow` and `robots.txt`
     disallows the prefixes (defence in depth; `robots.txt` alone does not stop indexing of a linked URL).
- Seller and admin apps: a missing robots policy would let login and console pages be indexed; their root layouts set
  `noindex,nofollow` for every route, and `robots.txt` on the buyer origin cannot cover their separate hosts.
- Authz rule: all of this is anonymous and read-only. Metadata uses the public gateway client and never the visitor's
  session, so it is identical for every visitor and a logged-in cookie changes nothing.
