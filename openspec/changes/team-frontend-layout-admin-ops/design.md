## Decisions

- Facades only: surfaces are the `@/components/ui` `Card` (with `CardHeader`/`CardFooter`), `Badge`, `Table`, `Empty`, `Link` and `Button`; layout uses tailwind with semantic token classes, so check-tokens stays green and the cockpit no longer carries raw palette classes.
- Nav at 375 px: a single horizontally scrolling row (`overflow-x-auto`, items `shrink-0 whitespace-nowrap`) instead of a menu. All eight link names stay visible to tests and screen readers without a toggle, and the scroll stays inside the nav, so the document never overflows. No active-link state: it would need `usePathname` in a client component, and several existing tests mock `next/navigation` without it.
- Below `sm` the shell hides the user name (it stays in the DOM for `admin-user`) and the logout button does not wrap, so the brand, title and logout fit on one row at 375 px.
- Cockpit headings stay `h2` inside the card headers (custom heading in `CardHeader` rather than `CardTitle`, which renders `h3`), so the outline goes h1 then h2.
- Texts read by tests and platform-e2e are kept verbatim: `Total Gateway Throughput`, `Average gRPC P95 Latency`, `RED Metrics Radar`, `Hàng chờ trống`, `Trang sau`, the button names and the link names. The metrics grid is still the first `div.grid` on the page (cockpit page object).
- Tables wider than the viewport scroll inside the platform `Table` wrapper; the moderation title and service name columns keep a minimum width so rows stay readable on phones.

## Risks

- Visual review covered only the states that render without the gateway (cockpit with no data, moderation queue error), via a fake unverified session cookie that the middleware passes through. The filled tables were checked only through the vitest renders.
- `/admin/cases?tab=queue` changes look together with `/admin/listings`, because both use `ModerationQueueView`.
