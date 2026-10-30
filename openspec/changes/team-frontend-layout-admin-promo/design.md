## Decisions

- Facades only: surfaces are the `@/components/ui` `Card`, `Link`, `Button`, `Table`, `Descriptions`, `Alert` and `Modal`; layout uses tailwind with semantic token classes, so check-tokens stays green.
- `Link` from the facade replaces `LinkButton` where a full-width-on-mobile action is needed, because `LinkButton` does not forward `fullWidth`; accessible names stay the same.
- Mobile first: one column below `md` for forms; header actions and form actions stack full width below `sm`; every control keeps `min-h-touch` (44 px).
- The voucher discount-type control stays a native `select` (the facade set has no select) but uses the same shape as the platform field (`rounded-xl px-4 py-3 text-base min-h-touch`).
- Back links use `ghost` inside a `-ml-4` wrapper so the text lines up with the title while the hit area stays 44 px.
- Long values (voucher code in the title) wrap with `break-all` so a 375 px viewport never scrolls horizontally; tables keep the platform horizontal scroll inside their card.

## Risks

- Inline code links inside table rows stay inline text links (the table row is the target context), so they are shorter than 44 px.
- Visual review covered only the states that render without the gateway (login redirect and route errors), because the gateway was not running during the check.
