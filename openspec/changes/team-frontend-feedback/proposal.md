## Why

C1b and C2 moved Button, Card, Input and Checkbox onto `@fe-platform/ui`. C3 does the same for the feedback components: the buyer app still renders its own toast stack and its own Alert (Alert, AlertClient, alertView), so toasts and inline messages are styled and announced differently from the platform.

## What Changes

- `@fe-platform/ui` and `@fe-platform/tokens` (minor): new `Alert` (tones info, success, warning, danger; title, description, optional action; `closable` with `closeLabel` and `onClose`). `danger` renders role=alert, the other tones role=status; it is server-safe unless `closable`. `ToastProvider` accepts `label`, `dismissLabel` and `position` (`top` or `bottom`); toast messages accept `tone` (danger is role=alert, others role=status) and `duration` (0 keeps the toast until dismissed). Tokens add the `alert` map and toast `viewport`, `tone` and `body`.
- `team-frontend/src/components/ui/Alert.tsx` becomes a facade (`type` info/success/warning/error maps to the platform tone; `className` is gone). `AlertClient.tsx` and `alertView.tsx` are deleted.
- `team-frontend/src/components/ui/ToastProvider.tsx` becomes a facade over the platform provider: `useToast()` (`showToast`, `success`, `error`, `info`), the global `toast` helper and the `app:toast` window event keep working; toasts stay top right with a 3.5 s default.
- The three Alert call sites that passed `className` (seller wallet, order timeline, `ErrorState`) wrap the Alert in a div.
- `team-frontend/tailwind.config.ts` scans the vendored `@fe-platform/tokens` class maps so platform component classes are generated.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/components/ui/{Alert,ToastProvider,DataState}.tsx`, two call sites, `ToastProvider.test.tsx`, `tailwind.config.ts`, `team-frontend/vendor/*.tgz`, `team-frontend/package-lock.json`, `fe-platform/packages/{tokens,ui}`, `fe-platform/.changeset`, `fe-platform/docs/agents/catalog.json`.
- Visible changes: alerts and toasts are white cards with a tone-coloured border and the airy radius; the toast close control is a 44 px "Đóng" button instead of a small ✕.

## Non-goals

Select, Table, Link, Dialog swaps; dark theme; moving `tones.ts` users (Tag, Badge, Result) onto the platform; server actions or routes.

## Non-functional requirements

`npm run check` green in `fe-platform` and `team-frontend`; `npm run build` green; e2e selectors keep working: error alerts and error toasts are role=alert, info/success/warning alerts and success/info toasts are role=status, and the toast text appears once per role.
