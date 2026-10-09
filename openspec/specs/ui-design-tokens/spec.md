# ui-design-tokens Specification

## Purpose
Defines the design-token contract of `team-frontend`: three tiers (primitive, semantic alias, component),
modelled on Ant Design's Seed/Map/Alias token derivation and on `platform-core/docs/UI_SYSTEM_DESIGN.md` §2,
and the lint that keeps component code on tokens.

## Requirements

### Requirement: Tier 1 primitive tokens are declared once in the Tailwind config

`tailwind.config.ts` SHALL declare the primitive scales and nothing else SHALL introduce new raw values:
brand `primary` 50–950 with `DEFAULT` `#ee4d2d`, a neutral scale 50–900, accents `danger` (`#d0011b`),
`promo` (`#ffbe00`, light `#ffe97a`) and `success` (`#00bfa5`), the 4px spacing scale, radius `lg` 8px,
`xl` 12px, `2xl` 16px, the shadows `preline-card` and `preline-hover`, and a type scale limited to 12, 14,
16, 20 and 24 px for UI text plus 30 and 36 px display sizes reserved for hero and page-title headings.

#### Scenario: The brand scale resolves from the config

- **WHEN** a component uses `bg-primary-500`
- **THEN** the computed background colour is `rgb(238, 77, 45)`

#### Scenario: Text below 12px is not available

- **WHEN** the built CSS is searched for a font-size below 12px generated from the type scale
- **THEN** none is found

### Requirement: Tier 2 semantic aliases are CSS variables mapped into Tailwind

`src/app/globals.css` SHALL define the semantic aliases as CSS custom properties that reference Tier 1
values, and `tailwind.config.ts` SHALL expose them as colours: `action-primary`, `action-primary-hover`,
`surface-page`, `surface-card`, `surface-muted`, `border-subtle`, `border-strong`, `text-primary`,
`text-secondary`, `text-disabled`, `text-inverse`, `danger`, `promo`, `success`, `focus-ring`. Component
code SHALL prefer aliases over primitives for colour.

#### Scenario: Changing an alias restyles every user of it

- **WHEN** `--color-action-primary` is overridden on `:root` in a test page
- **THEN** a primary `Button` and a sale `PriceTag` both render with the overridden colour, with no
  component code changed

### Requirement: A token lint rejects raw values in component code

`scripts/check-tokens.mjs` SHALL scan `src/**/*.tsx` (excluding `src/generated/**`) and exit non-zero when
it finds a raw hex colour, an `rgb(`/`hsl(` literal, an arbitrary Tailwind value (`<utility>-[...]`) or an
inline `style` colour, printing `file:line` and the offending token for each. `npm run check` SHALL run it.
An explicit allow-list comment `// tokens-allow: <reason>` on the same line SHALL exempt a single line.

#### Scenario: A new arbitrary value fails the gate

- **WHEN** a developer adds `className="text-[9px]"` to any component and runs `npm run check`
- **THEN** the check fails and names that file, line and `text-[9px]`

#### Scenario: The current tree is clean

- **WHEN** `npm run check` runs on the branch after this change
- **THEN** the token lint reports 0 violations

### Requirement: The app has Exception and loading shells

Following the Ant Design Exception (404/500) and Skeleton patterns, `src/app/not-found.tsx` SHALL render a
404 result with a link home, `src/app/error.tsx` SHALL render a recoverable error result with a retry
action, and `src/app/loading.tsx` SHALL render a skeleton that reserves the layout of the page shell.

#### Scenario: Unknown route shows the 404 result

- **WHEN** a user opens `/does-not-exist`
- **THEN** the page shows a "not found" result with a visible link back to `/`, and the HTTP status is 404

#### Scenario: A thrown page error is recoverable

- **WHEN** a server component throws during render
- **THEN** the error result is shown with a "Thử lại" (retry) button that re-renders the segment

### Requirement: Server Actions share one result type

`src/lib/action-result.ts` SHALL export `ActionResult<T> = { ok: true; data?: T } | { ok: false; error: string }`
plus helpers `ok(data?)` and `fail(error)`, as the single result shape required by `UI_SYSTEM_DESIGN.md` §5.
Each phase change SHALL migrate only the actions it owns: cart and order actions belong to
`ui-phase-cart-checkout`, address/auth/account actions to `ui-phase-account`, listing/review/Q&A actions to
`ui-phase-product-detail`, seller actions to `ui-phase-seller`, order-detail actions to `ui-phase-orders`.

#### Scenario: A failed action is typed as an error

- **WHEN** a Server Action returns `fail("Hết hàng")`
- **THEN** TypeScript narrows on `ok === false` to a value whose `error` is `"Hết hàng"`, and `data` is not
  accessible on that branch
