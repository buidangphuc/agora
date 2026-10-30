## Decisions

- One platform Link with variants instead of a separate LinkButton export: a button-shaped link must stay an `<a>` (navigation without JS, `get_by_role("link")` in platform-e2e), so `Link` borrows the Button token maps (`button.base`, `button.variant`, `button.size`, `fullWidth`) rather than nesting a Button.
- `subtle` sets no text colour and no underline, only a link-coloured hover and the focus ring, so footer, back and title links keep the colour of their container. Where the original link set its own colour (`text-text-secondary`, `text-text-primary`) the colour moves to the wrapper `span`.
- `className` stays banned on the platform Link; per-site layout and typography move to a wrapper `span`, as C1b did with wrapper divs.
- Size mapping: old `px-5 py-2.5 text-base` links become `md`; `text-sm` link buttons become `sm`; `text-xs` link buttons become `xs`. `w-full sm:w-auto` becomes `fullWidth="mobile"`.
- The facade keeps the team variant names (`outline`, `white`) that Button already accepts, so link and button call sites read the same.
- Next client navigation: the platform Link still renders `next/link`, passing `href`, `prefetch`, `replace`, `scroll`, `onClick` and `data-*` through.

## Risks

- Text links lose a few per-site details (`hover:underline` becomes always underlined for `inline`; `active:scale-95` and `shadow-sm` on button links follow the platform tokens instead).
- `truncate` on a wrapper around an inline `<a>` needs the wrapper to be a block, so wrappers that truncate get `block`; the CollectionsManager row wrapper is a flex container and may no longer show an ellipsis.
- Button-shaped table actions are now 44 px high, which makes seller table rows taller.
