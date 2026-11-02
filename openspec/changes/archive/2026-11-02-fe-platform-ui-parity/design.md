## Context

`team-frontend/src/components/ui/Button.tsx` and `Card.tsx` carry six variants, four sizes, icons, a loading overlay, hover lift and five card parts. The platform's `Button` and `Card` are class maps with no `className` or `style` by design (`props.typecheck.tsx` fails the build if one is accepted). A survey of 144 `<Button>` sites found 34 passing `className`: mostly `w-full`, `min-w-*`, `shrink-0`, a few colour overrides (`text-danger`, `border-action-primary`).

## Decisions

1. Keep the className ban. Width becomes `fullWidth`; the colour overrides are handled by `danger` and by C1b dropping or restyling them with the variants that exist.
2. Variant mapping used by the C1b facade: `primary` to `primary`, `outline` and `white` to `secondary`, `ghost` to `ghost`, `danger` to `danger`; the old flat `secondary` also maps to `secondary` (a border is added, accepted delta). So the platform adds only `danger`.
3. `danger` needs a surface colour the platform lacks. Add semantic `action-danger` (light `danger-dark`, dark `danger`) and `action-danger-hover` (light new primitive `danger-darker` #991b1b, dark `danger-dark`); white text on each stays at or above 4.5:1 and is added to `pairs.mjs` so the contrast gate checks it.
4. Size `xs` keeps `min-h-touch`: the 44 px floor applies to every size, so `xs` only shrinks padding and text. This grows the 13 `xs` and 82 `sm` buttons in `team-frontend`; C1b records it as an intended change.
5. `Card` base loses `p-6`; `padded` (default true) adds it back, so existing platform usage renders the same. `padded={false}` is the flush surface the CardHeader and CardContent parts need. `hoverable` changes border and shadow only: the platform spacing scale has no `0.5`, so the old translate lift is dropped. `loading` replaces children with three muted pulse bars and sets `aria-busy`.
6. Icons are `ReactNode` props wrapped in an `inline-flex shrink-0` span, spinner is a token class (`animate-spin` border ring) shown beside the label, label kept in flow so width does not jump.
7. `ref` already flows through `...rest` (React 19 passes `ref` as a prop and `NativeProps` uses `ComponentPropsWithRef`), so no `forwardRef` is added.

## Risks / Trade-offs

- [Wider platform API] -> every addition is typed, tested, in the state matrix and the catalog; nothing accepts raw classes.
- [Tokens gain a primitive] -> additive, minor changeset; the contrast gate fails if a pair drops below AA.
