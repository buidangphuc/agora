# Frontend guidelines v0

Short on purpose. Revise from what the P5 five-screen experiment shows going wrong.

## Hierarchy
- One primary action per view; secondary beside it; danger only for destructive actions.
- Never two primary buttons in one view. Navigation is a link, not a button-styled link.

## Spacing and layout
- Use the spacing, radius and type scale from tokens only: no arbitrary values, no raw colours.
- Controls use the control radius, cards the card radius; do not mix within a screen.
- Page content sits in the page container; forms are a single column.

## States
- Every interactive component shows default, hover, focus-visible, active, disabled, loading and error where they apply. Do not improvise a missing state; add it to the component.

## Empty and error states
- Say what happened and the next step, in plain words. No blame, no raw error codes; show the request id for support.

## Boundaries
- Feature code imports UI only from `packages/ui` and values only from `packages/tokens`. Use the existing component before writing a control; a missing variant is added to the component, not styled around it.
- Components never call the network. Data loads in server components or `lib/` functions through `@fe-platform/api-client`; screens receive plain view models, never proto types. Failures are `ApiError`; `not_found` becomes `notFound()`.
- Public routes must not import internal-only code.
- A component used by one feature stays in that feature until a second use appears.

## Never
- Add a lint or type disable without a stated reason. Fixing beats silencing.
- Hand-edit generated code.

## Feedback loop
Run `npm run check` before reporting done. See `project.md`.
