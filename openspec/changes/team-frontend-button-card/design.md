## Decisions

- Facade over rewrite: call sites keep importing `@/components/ui/Button` and `Card`, so the swap is one file plus the sites that used `className`; the old implementation is deleted, not kept beside it.
- Variant mapping: `primary`, `ghost`, `danger` map 1:1; `secondary`, `outline` and `white` map to platform `secondary` (bordered surface). The old flat grey `secondary` gains a border.
- `text-danger` on ghost or outline buttons becomes `variant="danger"`; `border-action-primary text-action-primary` becomes `secondary`.
- `w-full` becomes `fullWidth`; `w-full sm:w-auto` becomes `fullWidth="mobile"` (a small platform addition, cheaper than 9 wrappers). `min-h-*`, `min-w-*`, `shrink-0` and `underline` are dropped.
- Card facade defaults to `padded={false}`; padding-only `className` (`p-3` to `p-6`) becomes `padded` (p-6). Placement classes move to a wrapper element; inner layout classes move to a div inside the Card or CardContent; `CardHeader` stacked layouts wrap their children in one div.
- Lockfile trap: npm reuses a cached tarball when the lockfile integrity is stale, so after `npm run vendor` the `integrity` of the four `@fe-platform/*` entries must be rewritten from the new tarballs before `npm ci`.

## Risks

- Loading width: the spinner no longer overlays the label, so non-full-width buttons change width while loading. Accepted; `PlaceOrderForm` and `CheckoutSteps` lose their `min-w-40`.
- Selected-card emphasis that used a conditional border class is rebuilt with a wrapper ring.
