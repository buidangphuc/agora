# [W2-T2] frontend — voucher checkout + flash-sale UI

## Role
SE   # Next.js App Router, Connect-ES via gateway module

## Objective
Buyer can enter a voucher code at checkout and see the discount applied; flash-sale listings show sale price + live remaining-stock meter (LiveFlashSaleStock). Seller can create a voucher/campaign from the vouchers page.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/vouchers/**            (edit)
- team-frontend/src/features/voucher/**        (edit)
- team-frontend/src/components/flashsale/**    (create — FlashSale badge/meter component rendered by the listing slot)
- team-frontend/src/lib/gateway/promotion.ts   (create — server-only gateway client)

## Read-only dependencies
- team-frontend/src/generated/** (promotion stubs, Wave 0)
- listing/[id]/page.tsx + checkout/page.tsx slots (Wave 0 — already render your components; DO NOT edit)
- src/lib/gateway/vouchers.ts, track.ts (existing patterns)

## Contracts
Server-only gateway module builds a per-request client from the httpOnly `session` cookie. Checkout: call promotion ValidateAndReserve preview to show discount before submit; order submit carries voucher_code. LiveFlashSaleStock polls GetFlashSaleStock.

## Acceptance criteria
- [ ] Voucher input on checkout shows applied discount and reduced total; invalid code shows the reason.
- [ ] Flash-sale listing shows sale price + live remaining stock meter.
- [ ] No business logic in the page beyond calling the gateway module; typechecks/build pass.

## Review (gate — different agent)
Peer agent against SE rubric; frontend-talks-only-to-gateway boundary (contract-boundary-reviewer).

## Verify
```bash
# in Docker or node: (cd team-frontend && npm run typecheck && npm run build)
```

## Out of scope
- Do NOT edit listing/[id]/page.tsx or checkout/page.tsx shells (Wave 0). Do NOT edit other features' dirs. No direct service calls (gateway only).
