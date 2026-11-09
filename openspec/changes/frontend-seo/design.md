## Context

`team-frontend` is Next.js 15 App Router, SSR, `force-dynamic` on most public pages. Route group `(shop)` mixes public
and private segments under one layout, so per-segment layouts are the only place to attach a robots policy without
moving routes. Metadata merging in Next is shallow per top-level key: a page's `metadata` replaces the same key from a
layout and keeps the others, so `login/page.tsx` (`referrer`) and a `login/layout.tsx` (`robots`) coexist.

## Decisions

- Title template: root `title.template = "%s | Agora"`. Pages return the bare page name; hand-written suffixes
  (" | Kênh người bán", " | Sàn Thương Mại Điện Tử", " | Marketplace") are removed from the pages of all three
  frontends so the template cannot double them. Seller and admin set the same template in their root layouts; a root's
  own default title is written out in full because a template never applies to the segment that defines it. A page that wants the whole title uses `title: { absolute }` (home does not; it uses the
  root default).
- No `openGraph` or `alternates` in the root: they would be inherited by the 404 page, which must carry no OG tags.
  `pageMetadata()` sets them per page.
- Origin: `NEXT_PUBLIC_SITE_URL`, trailing slash trimmed, default `http://localhost:3000`, read through `siteUrl()`
  (server only use). Never derived from request headers.
- Canonical: PDP `/listing/<id>`; shop `/shop/<id>`; category `/search?category=<id>`; plain search `/search`; home
  `<site>` with no trailing slash (C19). All other query parameters (sort, filters, page) are dropped so variants consolidate. `q=` pages carry no
  canonical and `robots: {index:false, follow:true}`.
- Description: PDP uses the listing description, whitespace collapsed, truncated to 160 characters on a word boundary;
  if empty, `Mua <title> giá <price> ₫ trên Agora.` Shop uses the tagline, else `Cửa hàng <name> trên Agora`. Category
  uses `Mua <category name> chính hãng, giá tốt trên Agora.` Home uses the root default.
- `og:image`: PDP `imageUrl` (first image); shop `bannerUrl` (the storefront has no logo field in the proto); resolved
  against `metadataBase` when relative. When there is no image the tag is omitted.
- Shop name: reuse the existing `shopLabel` helper with `displayName`.
- Request caching: `loadListing` already is `requestCache(getListing)`. Add `loadStorefront = requestCache(getStorefront)`
  and have both `shop/[id]/page.tsx` and its `generateMetadata` use it. Search metadata uses `loadCategories` (React
  `cache`). A loader failure yields default metadata, never an error.
- Private pages: new `layout.tsx` files (children passthrough) under `account`, `cart`, `chat`, `notifications`,
  `favorites`, `dev`, `login`, `register` exporting `metadata = { robots: { index: false, follow: false } }`, and the
  same export in `(checkout)/layout.tsx`. `referrerMetadata.test.ts` only parses `login/page.tsx`, which is unchanged.
- `robots.ts`: `rules: [{ userAgent: "*", allow: "/", disallow: [account, cart, checkout, seller, admin, login,
  register, chat, notifications, favorites, dev prefixes] }]`, `sitemap: absoluteUrl("/sitemap.xml")`, `host` omitted.
  `/api/` is also disallowed.
- Sitemap: `export const revalidate = 3600`. Entries: home; each category from `loadCategories`; listings from
  `SearchListings` with `status=published`, `sortBy=NEWEST`, `page_size=50`, walking `next_cursor` until 5,000 hits or
  the cursor ends (100 sequential requests for 5,000; the cursor is an opaque offset so pages cannot be fetched in
  parallel); shops from `SearchHit.seller_id` (new, from `discovery-seo-contract`), de-duplicated. The server clamps
  `page_size` to 50: `team-search/internal/rpc/search.go:24` (`maxPageSize = 50`) and `:169` (`pagination.Clamp`). The
  existing `searchListings` wrapper (`SEARCH_MAX_PAGE=20` pages of 24, `getListing` per hit) cannot serve this, so
  `src/lib/gateway/sitemap.ts` calls the search client directly and never calls `getListing`. A gateway failure is
  caught and the sitemap returns what it has (home at minimum), status 200. The 2 s figure is a target: a cold render of 5,000 listings is measured and the duration reported, not gated
  (about 20 ms per request would meet it).
- Favicon: `src/app/icon.svg` (App Router file convention) so no `public/` file is needed.

## Risks

- `NEXT_PUBLIC_*` is inlined at build time: a per-environment build arg is needed for real canonical origins; local
  default keeps e2e working.
- Adding layouts to private segments must not change the DOM; they return `children` only.

## Clarifications

- C18 (human, 2026-11-08): the listing page's `og:type=product` is emitted as a raw meta tag through metadata `other`; `openGraph.type` stays unset.
- C19 (human, 2026-11-08): the SEO e2e follows Next 15 defaults. Googlebot receives streamed metadata (no `htmlLimitedBots`), so the test parser skips SVG `<title>` and reads streamed tags. The root canonical is `[site]` without a trailing slash, and SEO-16 no longer lists `/dev/ui`.

## Seller and admin apps

- Root `metadata` in each gets `robots: { index: false, follow: false }` and `title: { default, template: "%s | Agora" }`.
  Pages that export `metadata` replace only their own keys, so the root robots policy is inherited. Admin keeps
  `referrer: "no-referrer"` in the root and in each page that already sets it; `referrerMetadata.test.ts` is unchanged.
- Unknown shop id: `getStorefront` yields null for it and for a seller without a storefront, and the shop page already
  renders for both; metadata falls back to the shop label, no `og:image`, canonical kept, status 200. No 404 is added
  because the two cases cannot be told apart without a new RPC.
- Listing loader outage: `getListing` throws on non-NotFound errors. `generateMetadata` catches and returns the root
  default metadata; the page's own error boundary behaviour is unchanged.
