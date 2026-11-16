# LOCK — seller-centre
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-13
spec: plans/seller-centre/00-spec.md (Locked 2026-11-13)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| seller-centre-contract | platform-core | [] | 1 | pass | pass | pass | ENGINE_DOWN | 8b8008504400 → c4dff7b1698c → 5cab20d26502 | [x] |
| domain-storefront-customisation | team-domain | [] | 1 | pass | pass | pass | ENGINE_DOWN | b790361fd8dd → 2d041a834e1b | [x] |
| domain-my-listings-filter | team-domain | [seller-centre-contract] | 1 | pass | pass | pass | ENGINE_DOWN | 3ecb6779f75a → e638fda9a09e | [x] |
| domain-bundle-integrity | team-domain | [] | 1 | pass | pass | pass | ENGINE_DOWN | cf9bc98fe270 → 34d6e8a42a2d | [x] |
| seller-shop-customisation-ui | team-frontend-seller | [domain-storefront-customisation, domain-my-listings-filter] | 1 | pass | pass | pass | ENGINE_DOWN | b5a0802622cc → a3ac6fcd3f7a | [x] |
| shop-page-featured-theme | team-frontend | [domain-storefront-customisation] | 1 | pass | pass | pass | ENGINE_DOWN | 670decbcf356 → 41aa0b0be502 | [x] |
| promotion-seller-self-service | team-promotion | [seller-centre-contract] | 1 | pass | pass | pass | ENGINE_DOWN | 88b5553a6c87 → 7ee9f55ff568 | [x] |
| seller-marketing-hub | team-frontend-seller | [promotion-seller-self-service, domain-my-listings-filter] | 3 | pass | pass | pass | ENGINE_DOWN | f847db14ecee → 43bc90e0d578 → f63dadbb97fd | [x] |
| seller-getting-started | team-frontend-seller | [seller-marketing-hub, seller-shop-customisation-ui] | 1 | pass | pass | pass | ENGINE_DOWN | fa2485d9f803 → a9c904c7f308 | [x] |
| order-bundle-checkout | team-order | [seller-centre-contract, domain-bundle-integrity] | 1 | pass | pass | pass | ENGINE_DOWN | 0c3095790822 → 39953b03466d | [x] |
| shop-bundle-purchase-ui | team-frontend | [order-bundle-checkout, shop-page-featured-theme] | 1 | pass | pass | pass | ENGINE_DOWN | ada306a5c726 → 3eedf4b93f73 | [x] |
| order-seller-list-paging | team-order | [seller-centre-contract] | 1 | pass | pass | pass | ENGINE_DOWN | 307b8332526a → f5a8f26deb30 | [x] |
| seller-server-paging-ui | team-frontend-seller | [order-seller-list-paging, domain-my-listings-filter] | 1 | pass | pass | pass | ENGINE_DOWN | a791d8a98a72 → e8ee9103c1e4 | [x] |
| seller-bulk-ship | team-frontend-seller | [seller-server-paging-ui] | 1 | pass | pass | pass | ENGINE_DOWN | 34c482424858 → 7a6993e5a823 | [x] |

decide (spec) = ENGINE_DOWN for every slice: scripts/asdlc/decide.py needs `--units`/`--out` and the default python3 lacks `yaml`; each spec-quality-reviewer applied its own checklist by hand.
Digest = `find openspec/changes/<slice> -type f | sort | xargs shasum -a 256 | shasum -a 256 | cut -c1-12`, taken 2026-11-13.
Digest `old → new` (2026-11-15, chore(seller-centre): converge FEATURES and spec citations): only VERIFIED BY citation lines in spec.md and tasks.md checkboxes changed; scenario wording unchanged.

## Re-approval 2026-11-16
batch re-approval after converge (decision 5)
approved-by: Bùi Đăng Phúc
date: 2026-11-16

Every slice digest moved from its 2026-11-13 lock value during build and converge. The new digest is the last value in the `digest` column above and was recomputed on 2026-11-16.

| slice | old (2026-11-13 lock, then build re-digests) | new (2026-11-16) | reason |
|---|---|---|---|
| seller-centre-contract | 8b8008504400 | c4dff7b1698c | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| domain-storefront-customisation | b790361fd8dd | 2d041a834e1b | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| domain-my-listings-filter | 3ecb6779f75a | e638fda9a09e | ticked tasks only |
| domain-bundle-integrity | cf9bc98fe270 | 34d6e8a42a2d | ticked tasks only |
| seller-shop-customisation-ui | b5a0802622cc | a3ac6fcd3f7a | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| shop-page-featured-theme | 670decbcf356 | 41aa0b0be502 | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| promotion-seller-self-service | 6f7920975566 → 88b5553a6c87 | 7ee9f55ff568 | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| seller-marketing-hub | e997b9518e3e → f847db14ecee → 43bc90e0d578 | f63dadbb97fd | VERIFIED BY citations, ticked tasks, and the buyer-without-listing.write scenario/proposal now follow the archived seller onboarding gate (user decision 2026-11-15, 08809f85f) |
| seller-getting-started | fa2485d9f803 | a9c904c7f308 | ticked tasks only |
| order-bundle-checkout | 0c3095790822 | 39953b03466d | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| shop-bundle-purchase-ui | b4e35900d5a9 → ada306a5c726 | 3eedf4b93f73 | VERIFIED BY citations in spec.md and ticked tasks; no requirement or scenario wording change |
| order-seller-list-paging | 664ebea0e750 → 307b8332526a | f5a8f26deb30 | VERIFIED BY citations, ticked tasks, and migration renumbered 0025 to 0026 in proposal and spec (user decision 2026-11-15) |
| seller-server-paging-ui | 4301e73500fb → a791d8a98a72 | e8ee9103c1e4 | VERIFIED BY citation for the listing.write denial scenario and ticked tasks; no requirement or scenario wording change |
| seller-bulk-ship | 34c482424858 | 7a6993e5a823 | ticked tasks only |

## Re-digest 2026-11-16 (decision 13)
decision 13 tick, approved 2026-11-16 by Bùi Đăng Phúc.

| slice | old | new | reason |
|---|---|---|---|
| seller-centre-contract | c4dff7b1698c | 5cab20d26502 | task 3.2 ticked after JVM stubs were generated locally (protoc 28.3, protoc-gen-grpc-kotlin 1.4.1; grpc-java 1.68.1 stubs not generated because its osx-aarch_64 binary is x86_64 and Rosetta is absent); no requirement or scenario wording change |

## Known exceptions
Accepted by Bùi Đăng Phúc on 2026-11-16 (decision 9). In the converge run, `scripts/validate_plan.py plans/seller-centre` reported these same-wave conflicts. Each one was the literal token `widened` from the old `widened (P1)` sub-heading in the packets, which validate_plan read as a path. Neither slice wrote a file the other slice owns. Since f0d7b6ea4 flattened those entries into the write-sets, `validate_plan.py plans/seller-centre` passes.
- wave 1: domain-my-listings-filter vs promotion-seller-self-service, and order-seller-list-paging vs promotion-seller-self-service
- wave 2: domain-storefront-customisation vs seller-server-paging-ui
- wave 4: order-bundle-checkout vs seller-marketing-hub
- wave 5: seller-getting-started vs shop-bundle-purchase-ui

## Build order (cross_change_check)
1. seller-centre-contract (sequential, contract first)
2. domain-bundle-integrity
3. domain-my-listings-filter
4. domain-storefront-customisation
5. order-bundle-checkout
6. order-seller-list-paging
7. promotion-seller-self-service
8. seller-marketing-hub
9. seller-server-paging-ui
10. seller-bulk-ship
11. seller-shop-customisation-ui
12. seller-getting-started
13. shop-page-featured-theme
14. shop-bundle-purchase-ui

Hot-file pairs that must not run in parallel (00-spec "Capabilities → slices"):
- `team-frontend-seller/src/features/seller/nav.tsx`: seller-marketing-hub, seller-getting-started
- `team-frontend-seller/src/app/seller/(dashboard)/page.tsx`: seller-getting-started, seller-server-paging-ui
- `team-frontend-seller/src/features/seller/listingsPage.ts`: seller-server-paging-ui, seller-shop-customisation-ui, seller-marketing-hub
- `team-frontend-seller/src/app/seller/orders/(list)/page.tsx`, `src/lib/gateway/orders.ts`: seller-server-paging-ui, seller-bulk-ship
- `team-frontend-seller/src/lib/gateway/promotion.ts`, `src/features/seller/actions.ts`: seller-marketing-hub, seller-shop-customisation-ui, seller-getting-started
- `team-frontend/src/app/(shop)/shop/[id]/page.tsx`: shop-page-featured-theme, shop-bundle-purchase-ui
- `team-gateway/internal/edge/` (promotion.go, cart.go): promotion-seller-self-service, order-bundle-checkout
- `team-domain/internal/rpc/listing.go`: domain-my-listings-filter, domain-bundle-integrity
- `team-order/internal/store/order.go`: order-bundle-checkout, order-seller-list-paging

## Cross-change check
OK (exit 0), 0 violations (initial run had 12; fixed by adding the initiative line and renaming duplicate scenario titles).

## Open issues / NEEDS_HUMAN
00-spec has no NEEDS_HUMAN items. Expected risk routing from 00-spec: proto (contract), payment (bundle discount in checkout totals), auth (new seller-scoped RPCs and gateway routes), pii (batch packing slips).

Escalated slices: none failed after 3 rounds. seller-marketing-hub reached round 3 (the limit); its round-3 author says the reviewer read a stale proto (`Voucher.active = 13`, `deactivated_at = 14` already exist) — confirm.

### Cross-slice decisions (change more than one spec)
1. Bundle money semantics (order-bundle-checkout, shop-bundle-purchase-ui, seller-centre-contract): is `CartItem.bundle_discount` the whole-line allocation or per unit, and is `Cart.subtotal` net of the bundle discount (with `bundle_discount_total` shown separately)? Both slices assume line total + net subtotal.
2. Bundle + flash sale: bundle lines use the plain listing price only (no flash-sale/advertised price, no variants), both for CreateBundle's price check and for checkout. Confirm.
3. Seller CreateCampaign on a foreign listing (promotion-seller-self-service, seller-marketing-hub): should team-promotion return PERMISSION_DENIED for a non-admin creating on another seller's listing? Without it the hub's "Foreign listing id is denied" e2e cannot pass. Also: does the gateway edge admit `listing.write` on CreateCampaign, and does the 8..64 `[A-Za-z0-9_-]` idempotency-key rule apply on the seller path?
4. Seller-side rendering of bundles on the seller's order (00-spec story 4): no slice owns it (shop-bundle-purchase-ui excludes it). Assign to order-bundle-checkout, seller-server-paging-ui, or drop.
5. ListMyListings page size: NFR says default 20, team-domain keeps today's 50 (max 100). Change to 20? And confirm today's max is already 100 so clamping does not change other callers.
6. Shared ListingPicker / `searchMyListingsAction` introduced by seller-server-paging-ui: seller-shop-customisation-ui and seller-marketing-hub must reuse them (names not aligned yet).
7. Featured picker / featured ids: PUBLISHED listings only (seller-shop-customisation-ui), and shop-page-featured-theme treats unset status as published. Confirm drafts cannot be featured.

### Per-slice decisions that change a spec
- seller-centre-contract: query limit of 100 counts runes (the behaviour slices say runes); `page.total = -1` means unknown per common.proto. Seller RPCs are not audited (00-spec says so; treated as settled).
- domain-storefront-customisation: accept a `banner_url` equal to the stored value even when off-bucket? Store an empty theme as empty, or normalise to `default`? If prod serves banners from a CDN host, that host must join the allowed prefix. Adds a batched listing read to the repository port (internal).
- domain-my-listings-filter: `DELETED` in statuses returns an empty page (not INVALID_ARGUMENT)? A malformed cursor returns INVALID_ARGUMENT? Index is `listings (seller_id, status, id)` (00-spec wrote `owner_id`). Does the 150 ms p95 include the exact filtered count?
- domain-bundle-integrity: foreign and unknown listing ids return PERMISSION_DENIED (enumeration-safe) vs INVALID_ARGUMENT for consistency with storefront? Is an owned but unpublished/deleted listing accepted in a bundle (00-spec defers the published check to AddBundleToCart)? Legacy bad bundle for B12 seeded by SQL fixture: acceptable? Overflow of the int64 price sum: add a rule or drop.
- seller-shop-customisation-ui: banner limits copied from the listing form (jpeg/png/webp, max 10 MB)? Should `getUploadUrlAction` also require `listing.write`? How does the foreign-featured-id e2e inject the id (direct server-action call, tampered form, pre-seeded storefront)?
- shop-page-featured-theme: allow the ListingService.GetListing fallback for featured ids outside the shop's search page (not in `uses`), or silently skip them? Theme palette (dark, festive) needs design approval. Theme key match case-sensitive? NFR "today's budget + 100 ms" has no baseline number.
- promotion-seller-self-service: admin CreateCampaign owner lookup is best-effort (lookup failure leaves `seller_id` empty for backfill, admin call still succeeds)? Seller RPCs reject an admin holding only `promotion.admin`: intended? Are p95 targets (150 ms lists, 100 ms stops) benchmarked or advisory?
- seller-marketing-hub: flash-sale form is listing-level only (no variant picker)? Voucher state (Đang chạy / Sắp diễn ra / Đã kết thúc / Đã tắt) derived in the app, or server-owned field? Confirm deleting the buyer VoucherManager and `createVoucherAction` in team-frontend (no admin flow uses them). Confirm the Vietnamese error copy.
- seller-getting-started: step "shop name" done when `display_name` is non-empty (slug fallback does not count)? 3 s per-source timeout and the "one source missing, other empty = chưa rõ" rule? Card still shown when every source is unknown?
- order-bundle-checkout: AddBundleToCart quantity 1..99? Precheck failure reuses the existing price-changed error after untagging lines (no new code)? Seller adding their own bundle refused only at checkout by the self-purchase rule? Split the unavailable-listing scenario per cause (unpublished, out of stock, foreign owner)? Also calls ListingService.GetListing (not in `uses`).
- shop-bundle-purchase-ui: out-of-stock/variant bundles flip to unavailable only after FAILED_PRECONDITION (no extra stock reads on render)? Dropped-bundle notice is client-side? Checkout bundle discount comes from cart data (no PreviewCheckout fields)?
- order-seller-list-paging: gateway JSON forwarder keeps an absent `page` as nil (legacy full list)? ListSellerOrders stays without a `listing.write` scope check? `status_counts` always lists all 5 statuses including zeros? Order id prefix case-insensitive, title search on `order_items.title` only?
- seller-server-paging-ui: cursor-only paging with "Trang đầu / Trang sau" and no page-N links (or ask for an offset field)? Search by recipient name/phone is dropped. "Cần gửi" becomes PENDING + PAID (counts prepaid PENDING orders; overdue note from first 100 rows). "Sắp hết hàng" stays over the first 20 rows.
- seller-bulk-ship: selection limited to the current page (50 cap unreachable at page size 20), or cross-page / larger page on the to-ship tab? Checkbox on every `isToShip` row (PAID, or PENDING with COD) or PAID only? 15 s budget is best-effort (unstarted orders show "Chưa xử lý, hãy thử lại", in-flight calls not aborted)? Print without `listing.write`: no slips, or redirect to `/forbidden`? Failure codes for cancelled/shipped orders assumed FAILED_PRECONDITION.

### Minor findings (authors fix without a human)
- spec_lint PERM-scenario warnings (contract, domain-my-listings-filter, seller-shop-customisation-ui, promotion-seller-self-service, seller-bulk-ship): authz covered in sibling requirements; add a cross-reference line.
- domain-storefront-customisation: reword the owner-scoped clause; add scenarios for the fallback banner prefix, a 2049-byte banner, and a trimmed 120-rune tagline.
- domain-my-listings-filter: add a cross-seller cursor reuse scenario.
- domain-bundle-integrity: tag tasks 2.1/2.2/2.4 with B13; name the reduced-scope e2e token fixture.
- shop-page-featured-theme: wording for unset status; add a `loadStorefront` failure test; split the Theme requirement.
- seller-marketing-hub: add the buyer-cannot-apply step to e2e task 8 or drop it; add one voucher INVALID_ARGUMENT scenario.
- shop-bundle-purchase-ui: distinct test for C13 checkout summary; name UNAVAILABLE/DEADLINE_EXCEEDED in C6; state that out-of-stock has no e2e; confirm the "Tiết kiệm 30.000 VND" format.
- seller-bulk-ship: fix the foreign-order e2e scenario title/body; add page-level verification for malformed ids; add an action-level not_attempted test.
- Proto fields used by consumer slices land with seller-centre-contract; initiative-build must merge it first.
