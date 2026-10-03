# UI System Design & Frontend Architecture

> **Single Source of Truth** for frontend visual language, component taxonomy,
> design tokens, and technical architecture in the Agora polyrepo platform.
> Mirrors backend architectural standards in `ARCHITECTURE.md` and `DATA_ARCHITECTURE.md`.

---

## 1. Design Principles (Product-Specific Point of View)

Design principles act as grammar rules to resolve tradeoffs and maintain aesthetic integrity.

| Principle | Meaning & Concrete Interface Rule | Anti-pattern to Reject |
|---|---|---|
| **1. Clarity over Visual Clutter** | Visual hierarchy and whitespace dictate focus. Use 12px/14px/16px baseline type scale. Reserve bright brand color (`#ee4d2d`) for primary calls to action, badges, and pricing. | Cramming 8px/9px micro-text everywhere, rainbow tags, or unspaced element borders. |
| **2. Unbreakable & Predictable Commerce** | Every transaction touchpoint (Add to Cart, Variant Selector, Checkout, RMA) must provide immediate feedback (loading spinner, optimistic feedback, or disabled state). | Silent clicks, ambiguous pricing breakdowns, or layout jumping during checkout. |
| **3. Token-First Guarantee** | No raw hex codes (`#123456`) or arbitrary values (`text-[9px]`, `p-[7px]`) in component code. Every style decision must resolve through the design token hierarchy. | Ad-hoc inline CSS utilities that drift across features. |
| **4. Zero Layout Shift (CLS = 0)** | Images, feeds, and banners must declare explicit aspect ratios (`aspect-square`, `aspect-2/1`). Dynamic data blocks must use skeleton fallbacks. | Content pushing down when images load or products pop into view. |

---

## 2. Three-Tier Design Tokens Architecture

Tokens are named design decisions stored as platform-neutral values, following the **W3C Design Tokens Community Group (DTCG)** model.

```
┌────────────────────────────────────────────────────────┐
│ Tier 1: Core Primitives                                │
│   color.orange.500 = #ee4d2d  |  space.4 = 16px        │
│   radius.lg = 12px            |  font.family = Inter   │
└───────────────────────────┬────────────────────────────┘
                            │ (referenced by)
┌───────────────────────────▼────────────────────────────┐
│ Tier 2: Semantic Aliases                               │
│   color.action.primary  = {color.orange.500}           │
│   surface.card          = {color.white}                │
│   border.subtle         = {color.neutral.200}          │
│   text.primary          = {color.neutral.900}          │
└───────────────────────────┬────────────────────────────┘
                            │ (bound to)
┌───────────────────────────▼────────────────────────────┐
│ Tier 3: Component Tokens                               │
│   button.primary.bg     = {color.action.primary}       │
│   card.ecommerce.radius = {radius.lg}                  │
│   price.tag.sale        = {color.action.primary}       │
└────────────────────────────────────────────────────────┘
```

### A. Tier 1 - Core Tokens (`tailwind.config.ts`)
The only place raw values live.
- **Primary Scale (Agora Brand)**:
  - `50: #fff5f2`, `100: #ffe8e1`, `200: #ffd4c7`, `300: #ffb5a0`, `400: #ff8566`
  - `500: #ee4d2d` (Brand Default)
  - `600: #d73211`, `700: #b52309`, `800: #941e0c`, `900: #7a1d0f`, `950: #430b05`
- **Neutral Scale** (`neutral-50` ... `neutral-900`): same values as Tailwind `gray`
  (`50 #f9fafb`, `100 #f3f4f6`, `200 #e5e7eb`, `300 #d1d5db`, `400 #9ca3af`, `500 #6b7280`,
  `600 #4b5563`, `700 #374151`, `800 #1f2937`, `900 #111827`).
- **Semantic Accents** (`accent.*`):
  - Mall/Danger: `#d0011b` (dark `#b00016`)
  - Promotion/Discount: `#ffbe00` / `#ffe97a`
  - Success/Freeship: `#00bfa5`
- **Spacing Scale (4px Baseline)**: Tailwind default (`1` = 4px, `2` = 8px, `3` = 12px, `4` = 16px, `5` = 20px,
  `6` = 24px, `8` = 32px, `12` = 48px) plus `18` = 72px.
- **Type Scale**: 12 / 14 / 16 / 20 / 24 px only (`text-xs` 12, `text-sm` 14, `text-base` 16, `text-lg`/`text-xl` 20,
  `text-2xl`..`text-5xl` 24). Nothing below 12px exists.
- **Elevation Shadows**:
  - `shadow-preline-card`: Soft resting card shadow (`0 1px 3px rgba(0,0,0,0.07)`).
  - `shadow-preline-hover`: Elevated card hover shadow (`0 10px 15px -3px rgba(0,0,0,0.08)`).
  - `shadow-xs` / `shadow-2xs`: hairline elevation.
- **Border Radius**:
  - Control/Button: `rounded-lg` (8px).
  - Card/Item: `rounded-xl` (12px).
  - Modal/Hero Banner: `rounded-2xl` (16px).
- **Layout sizes** (replace arbitrary values): `max-w-page` (1200px), `max-w-bubble` (75%), `max-w-bubble-wide`
  (85%), `h-chat` / `min-h-chat` / `max-h-chat`, `min-h-viewport-main`, `h-modal`.

### B. Tier 2 - Semantic Aliases (`src/app/globals.css`)
CSS variables on `:root` that reference Tier 1, exposed as Tailwind colours. Component code prefers these over
primitives. Because Tailwind prefixes the utility, the colour keys read `bg-action-primary`,
`text-text-primary`, `border-border-subtle`, and so on.

| Alias (CSS variable) | Tier 1 value | Tailwind use |
|---|---|---|
| `--color-action-primary` | `primary-500` `#ee4d2d` | `bg-action-primary`, `text-action-primary` |
| `--color-action-primary-hover` | `primary-600` | `hover:bg-action-primary-hover` |
| `--color-surface-page` | `neutral-100` | `bg-surface-page` |
| `--color-surface-card` | white | `bg-surface-card` |
| `--color-surface-muted` | `neutral-50` | `bg-surface-muted` |
| `--color-border-subtle` | `neutral-200` | `border-border-subtle` |
| `--color-border-strong` | `neutral-300` | `border-border-strong` |
| `--color-text-primary` | `neutral-900` | `text-text-primary` |
| `--color-text-secondary` | `neutral-600` | `text-text-secondary` |
| `--color-text-disabled` | `neutral-400` | `text-text-disabled` |
| `--color-text-inverse` | white | `text-text-inverse` |
| `--color-danger` | `accent.danger` `#d0011b` | `bg-danger`, `text-danger` |
| `--color-promo` | `accent.promo` `#ffbe00` | `bg-promo`, `text-promo` |
| `--color-success` | `accent.success` `#00bfa5` | `bg-success`, `text-success` |
| `--color-focus-ring` | `primary-300` | `ring-focus-ring` |

Redefining a variable restyles every consumer (a later dark mode is one block). Alias colours are plain
`var(...)`, so Tailwind opacity modifiers (`bg-action-primary/50`) do not apply to them; use a Tier 1 scale for that.

### C. Tier 3 - Component Tokens
Class maps inside `src/components/ui/*` (for example the `Button` variant map), referencing Tier 2 only.

### D. Mapping to Ant Design's token model
Ant Design 5 derives Seed to Map to Alias to Component tokens through CSS-in-JS. We keep the same shape on
Tailwind without depending on `antd`:

| Ant Design | Agora |
|---|---|
| Seed token (`colorPrimary`, `borderRadius`, `fontSize`, `sizeUnit`) | Tier 1 in `tailwind.config.ts` |
| Map token (`colorPrimaryBg`, `colorPrimaryHover`, ...) | Tier 1 scales 50-950 (pre-derived, not computed) |
| Alias token (`colorText`, `colorBgContainer`, `colorBorderSecondary`, ...) | Tier 2 CSS variables |
| Component token (`Button.primaryColor`, ...) | Tier 3 class maps in `src/components/ui/*` |
| Layout `Grid` (24 columns) / `Space` | Tailwind grid + `gap-*` on the 4px scale |
| Exception 403/404/500 | `src/app/not-found.tsx`, `src/app/error.tsx` |
| Skeleton | `src/app/loading.tsx` |

Enforcement: `npm run check` runs `scripts/check-tokens.mjs`, which rejects raw hex, `rgb()`/`hsl()`, arbitrary
`utility-[...]` values and inline style colours in `src/**/*.tsx`; `// tokens-allow: <reason>` exempts one line.

---

## 3. Component Taxonomy

Following the **Primitives → Components → Patterns → Templates** hierarchy (avoiding vocabulary disputes between atoms and molecules):

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ 1. PRIMITIVES (Atoms)                                                        │
│    • Button, Input, Badge, Card, Modal, PriceTag, Stepper, Tabs, Skeleton   │
├──────────────────────────────────────────────────────────────────────────────┤
│ 2. COMPONENTS (Molecules)                                                    │
│    • SearchBar (Input + Button + Suggestions)                                │
│    • ProductCard (Card + Image + PriceTag + Badges)                          │
│    • QuantityPicker (Input + Increment/Decrement Buttons)                   │
│    • ReviewRating (StarRating + Author + Sentiment Badge)                    │
├──────────────────────────────────────────────────────────────────────────────┤
│ 3. PATTERNS (Organisms & Domain Behaviors)                                   │
│    • FilterSidebar (Category, Price Range, Mall Facet)                       │
│    • OrderTimeline (Stepper Checkpoints + Saga Fallback)                     │
│    • CartGroupedByShop (Shop Header + Items + Voucher Selector)              │
│    • FlashSaleSection (Countdown Clock + Flame Progress Bar + 6-col Grid)    │
├──────────────────────────────────────────────────────────────────────────────┤
│ 4. TEMPLATES & LAYOUT SHELLS                                                 │
│    • Consumer Shell: Sticky Header, Mega Search, Bottom Nav, Toast Container │
│    • Merchant Shell: Collapsible Sidebar, Stat Metric Cards, Data Table Grid │
│    • Checkout Shell: Distraction-free Stepper Header, Secure Footer          │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 4. Behaviour & State Matrix

Handoff and implementation defects arise from guessing missing states. Every interactive component must define 6 standard states:

| State | Visual Treatment | Implementation Requirement |
|---|---|---|
| **Default** | Resting elevation, border-gray-200, neutral background | Semantic token color mapping |
| **Hover** | `-translate-y-0.5`, subtle shadow enhancement, primary border tint | Smooth CSS transition (150-200ms) |
| **Active / Pressed** | Scale-down (`active:scale-95`), darker background | Instant tactile feedback |
| **Focus** | 2px solid ring (`focus:ring-2 focus:ring-primary-500/20`), visible keyboard outline | Full Accessibility (A11y) navigation |
| **Disabled** | `opacity-50 pointer-events-none cursor-not-allowed` | `disabled` attribute + `aria-disabled` |
| **Loading** | Inline SVG spinner, preserved button width, disabled triggers | `isLoading` boolean prop |
| **Empty / Error** | Dedicated empty illustration + action link, error banner with recovery button | Contextual feedback |

---

## 5. Technical Frontend Architecture (Next.js 14 App Router)

### A. Server Components vs Client Boundaries
1. **Server Components (RSC) by Default**:
   - All page layouts, SEO metadata, Initial Product Feeds, Category Trees, and Server-Side Filter reads run as Server Components.
   - Zero client JS payload for static displays; fast Time to First Byte (TTFB).
2. **Client Components as Leaf Islands**:
   - Isolated only at interactivity boundaries: `CartQuantityButton`, `AddToCartButton`, `SearchBarAutocomplete`, `VariantSelector`, `ModalTrigger`.
   - Never mark an entire page as `"use client"` unless strictly necessary.

### B. State Management Strategy
- **Global & Search State ➔ URL SearchParams**:
  - `?q=query&category=cat_id&sort=price_asc&page=2`.
  - Enables browser back/forward history, shareable URLs, and server cache invalidation.
- **Transactional State ➔ Server Actions**:
  - Mutations (Add to cart, place order, update address) invoke Next.js Server Actions calling the Edge Gateway Connect-ES client.
  - Returns structured results: `{ ok: boolean, error?: string, data?: T }`.
  - Triggers Next.js `revalidatePath` to refresh server cache automatically.

### C. Performance & Image Handling
- Images must always be served via `getImageUrl()` helper with fallback placeholders.
- Strict `aspect-square` or predefined aspect ratio containers prevent Cumulative Layout Shift (CLS).
- Lazy loading enabled by default on below-the-fold feeds (`loading="lazy"`).

---

## 6. Phase-by-Phase Screen System Mapping

The Agora platform spans 5 full user lifecycle phases:

```
[Phase 1: Discovery] ──▶ [Phase 2: Product Detail] ──▶ [Phase 3: Cart & Checkout]
        │
        ├──▶ [Phase 4: Order Fulfillment & Timeline]
        │
        └──▶ [Phase 5: Merchant Center (Seller Cockpit)]
```

| Phase | Core Pages & Views | Key Design System Components |
|---|---|---|
| **Phase 1: Discovery** | `/` (Home), `/search` (Catalog & Facets), `/vouchers` | `ListingCard`, `SearchBar`, `CategoryBar`, `FlashSaleSection`, `FilterSidebar` |
| **Phase 2: Product Detail** | `/listing/[id]` | `ImageGallery`, `VariantSelector`, `PriceTag`, `ShopHeaderCard`, `Descriptions`, `ReviewSection` |
| **Phase 3: Cart & Checkout** | `/cart`, `/checkout`, `/checkout/pay/[id]` | `CartView`, `VoucherModal`, `AddressSelectorModal`, `PaymentOptionsGrid` |
| **Phase 4: Order Fulfillment** | `/account/orders`, `/account/orders/[id]` | `OrderTimeline` (Stepper), `Result` (Success/Failed), `ReturnRequestSection`, `OrderStatusBadge` |
| **Phase 5: User Settings** | `/account/addresses`, `/account/security`, `/account/verification` | `Tabs`, `Descriptions`, `Input`, `Badge` (KYC Verified / Pending) |
| **Phase 6: Merchant Workplace** | `/seller` (Dashboard), `/seller/new` (Studio), `/seller/orders` | `SellerLayout` (Sidebar), `Statistic` (KPIs), `ProductTable`, `InventoryForm` |

### Ant Design Pattern Mapping (Alibaba / E-Commerce Standard)

To avoid heavy `@ant-design` runtime dependencies while adopting its full enterprise phase coverage:

| Ant Design Pattern | Purpose | Agora Core Implementation |
|---|---|---|
| `Search / Card List` | High-density product discovery grid | `ListingGrid` + `ListingCard` (1:1 image, preline hover) |
| `Filter Table / Buckets` | Faceted search with count indicators | `FilterSidebar` + `SortBar` |
| `Step Form` | Multi-step transaction wizard | `Stepper` (Horizontal Checkout: Address → Ship → Pay → Confirm) |
| `Descriptions` | Structured Key-Value specs & metadata | `src/components/ui/Descriptions.tsx` |
| `Result` | Post-action state (Order Success, Payment Fail) | `src/components/ui/Result.tsx` |
| `Statistic` | Executive KPI cards with trends & prefixes | `src/components/ui/Statistic.tsx` |
| `Timeline / Steps` | Sequential shipment & Saga checkpoints | `OrderTimeline.tsx` (Vertical Stepper) |
| `Workplace Dashboard` | Merchant center overview & fast actions | `app/seller/page.tsx` + `SellerLayout` |

---

## 7. Governance & Contribution Workflow

1. **New Component Rule**:
   - Before building a new ad-hoc UI element, check `src/components/ui/`.
   - If a visual pattern is used in 3 or more places ("Three-uses rule"), it must be extracted into `src/components/ui/` with typed variants and sizes.
2. **Token Modification Rule**:
   - Any new color, spacing, or radius must be declared in `tailwind.config.ts`. No raw arbitrary brackets like `bg-[#abc]`.
3. **Verification Gate**:
   - `npm run check` (Biome linting + `tsc --noEmit` + Vitest) must pass on every PR touching `team-frontend`.
