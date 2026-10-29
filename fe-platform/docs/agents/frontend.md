# Frontend guidelines v1

Components, props and token names: `docs/agents/catalog.json` (generated; `npm run catalog`).

Short on purpose. v1 changes come from `docs/experiments/p5-five-screens.md`; finding numbers in brackets.

## Hierarchy
- One primary action per view; secondary beside it; danger only for destructive actions.
- Never two primary buttons in one view. Navigation is a `Link`, never a button-styled link and never a bare `next/link` or `<a>`, which render like body text [1]. Button is for actions on the current page.

## Spacing and layout
- Use the spacing, radius and type scale from tokens only: no arbitrary values, no raw colours.
- Controls use the control radius, cards the card radius; do not mix within a screen.
- A page is `<main className="flex flex-col gap-4 p-6">` with one `<h1 className="text-2xl">`; use `gap-3` only inside a form block. There is no page-container component [2]. Forms are a single column.
- A form that carries personal data (name, address, phone, email) uses POST or a server action, never a GET form or the query string [3]. Validate on the server; native `required` alone is not validation [5].

## States
- Every interactive component shows default, hover, focus-visible, active, disabled, loading and error where they apply. Do not improvise a missing state; add it to the component.

## Empty and error states
- Say what happened and the next step, in plain words. No blame, no raw error codes; show the request id for support.

## Boundaries
- Feature code imports UI only from `packages/ui` and values only from `packages/tokens`. Use the existing component before writing a control; a missing variant is added to the component, not styled around it.
- Components never call the network. Data loads in server components or `lib/` functions through `@fe-platform/api-client`; screens receive plain view models, never proto types. Failures are `ApiError`; `not_found` becomes `notFound()`.
- `templates/app/app/(public)/shop` is a demo with hardcoded products, cart maths in `lib/shop.ts` and no order call. Copy its look, never its data or pricing logic: real catalogue and order data come from the gateway, and the owning services stay authoritative for prices, totals and field rules.
- Public routes must not import internal-only code.
- A component used by one feature stays in that feature until a second use appears.

## Never
- Add a lint or type disable without a stated reason. Fixing beats silencing.
- Hand-edit generated code.

## Feedback loop
Run `npm run check` before reporting done. See `project.md`.
