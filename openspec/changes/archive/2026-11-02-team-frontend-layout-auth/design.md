## Context

The auth pages sit inside the shared `(shop)` layout (ConsumerShell), which this change must not touch. The fe-platform template screens (`fe-platform/templates/app/app/(public)/login`) use a narrow page with a header (`h1` + description) above one card holding the form.

## Decisions

- One page-specific wrapper, `AuthPanel`, shared by both routes so they cannot drift; it lives in `features/auth`, not `components/ui`, because only the auth pages use it.
- Width `max-w-md`, vertical rhythm `gap-6`/`md:gap-8`, `py-6`/`md:py-12`, header centred, `text-2xl`/`md:text-3xl` semibold heading: the template's narrow page adapted to a centred auth card.
- The page keeps a single heading (`h1`); the old `h2` greeting duplicated it, so its text becomes the description under the `h1`.
- The switch link uses the facade `Link` with `variant="secondary" size="lg" fullWidth`, giving a 44 px target with the same accessible name the tests and e2e use.
- Only semantic token classes (`text-text-primary`, `text-text-secondary`, `border-border-subtle`).

## Risks

- The route `loading.tsx` still shows the shared `AuthCardSkeleton` (`max-w-sm`), so the skeleton is slightly narrower than the loaded page; changing the shared skeleton is out of scope.
