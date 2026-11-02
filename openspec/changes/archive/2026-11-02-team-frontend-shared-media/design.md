## Decisions

- Drawer is built on `@radix-ui/react-dialog`, which the platform already depends on, instead of moving `useDialog`. Radix gives focus trap, Escape, overlay dismiss and scroll lock. Radix only returns focus to its own Trigger, so the Drawer records the focused element in `onOpenAutoFocus` and focuses it again in `onCloseAutoFocus`. The return happens after a timeout, so tests that use `fireEvent` wait for it.
- Untitled drawers render an `sr-only` Radix Title from `label` (team-frontend passes "Ngăn kéo"), so the accessible name is unchanged and Radix does not warn. Without a description the content passes `aria-describedby={undefined}`.
- The overlay is a Radix Overlay, not a "Đóng" button. Clicking it still closes the drawer; the close button is the only "Đóng" button now.
- Scroll lock comes from Radix (`data-scroll-locked` on `body`) instead of an inline `overflow: hidden`.
- `Image` `className` call sites used `rounded-lg`, `rounded-none` and `rounded-b-none`; they map to `radius`. `rounded-b-none` (ListingCard) becomes `none` because the token test only accepts radius steps and the Card already clips the top corners.
- Avatar `lg` and `xl` keep 56 and 80 px: spacing steps 14 and 20 are added instead of shrinking them to the existing scale.
- Spacing and font sizes outside the preset are snapped: `gap-1.5` and `mr-0.5` become `gap-1`, `px-1.5` becomes `px-2`, `text-lg` becomes `text-xl` (the same 20 px in team-frontend).
- `formatPrice` keeps its `vi-VN` output and unused `_currency` parameter so `LiveFlashSaleStock` compiles unchanged.
- The catalog test compares the Button size union as a set because TypeScript orders union members by creation, and the new exports create `"sm" | "md" | "lg"` before Button.

- team-frontend Tailwind `content` now also scans `node_modules/@fe-platform/ui/src` and `@fe-platform/tokens/src/tokens.mjs`. Before, classes used only inside the platform (for example `bg-surface-overlay`, `aspect-4/3`, `object-contain` once `aspect.ts` and `ImageClient.tsx` moved) were missing from the built CSS.

## Risks

- Other promotion branches may add the same `content` entries; the merge keeps one copy.

- The discount badge loses its indigo tint until a tint token exists.
