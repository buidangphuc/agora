# [W2-T4] frontend — wishlist collections + richer reviews UI

## Role
SE

## Objective
User can create named collections, add/remove listings, and view a collection. Review UI supports photos, a "Helpful" button with count, a verified-purchase badge, and a shop rating summary.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/favorites/**          (edit — collections view)
- team-frontend/src/features/engagement/**    (edit — collections)
- team-frontend/src/features/review/**        (edit — media, helpful, verified badge, shop summary; rendered by listing slot)
- team-frontend/src/lib/gateway/engagement.ts (edit — collections + review RPCs)

## Read-only dependencies
- team-frontend/src/generated/** (engagement stubs, Wave 0)
- listing/[id]/page.tsx slot (Wave 0 renders Reviews + WishlistButton components; DO NOT edit the shell)
- existing engagement gateway module + favorites page

## Contracts
Server-only gateway module. Collections CRUD + items; review create with media_urls (upload via existing media flow); MarkReviewHelpful; GetShopRatingSummary on shop page.

## Acceptance criteria
- [ ] Create collection, add/remove item, view items.
- [ ] Review shows photos, Helpful button increments count once per user, verified badge when applicable, shop summary renders.
- [ ] Typechecks/build pass; gateway-only.

## Review (gate — different agent)
Peer agent against SE rubric.

## Verify
```bash
# (cd team-frontend && npm run typecheck && npm run build)
```

## Out of scope
- Do NOT edit listing/[id]/page.tsx or checkout shells (Wave 0). Do NOT edit other features' dirs. No notification UI (that's W2-T5).
