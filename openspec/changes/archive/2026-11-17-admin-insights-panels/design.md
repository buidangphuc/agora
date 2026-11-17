## Context

`/admin/cockpit` is a server page that gates on the decoded `admin` scope, calls `fetchCockpit` (REST `/api/admin/metrics`) and hands the result to the client `CockpitView`, which polls. The app reaches the gateway only through server-only helpers; Connect clients are built per request in `src/lib/gateway/client.ts`, which has no analytics client yet. The five RPCs are all admin-scoped at the gateway.

## Decisions

- Server components and Connect, not the metrics REST payload: the report RPCs are separate from `/api/admin/metrics`, so adding them to that payload would put gateway-side business shaping back in the proxy. The page awaits the five fetches in parallel next to `fetchCockpit`; the panels are not polled (reports move daily, not per second).
- Query-parameter state instead of client state: selectors are plain GET forms (`<form method="get">`), the server parses and clamps. No client JS for the panels beyond what `Table` already uses, so the numbers cannot drift from the server render.
- Result union per fetch, mirroring `CockpitResult` and `reports.ts` (`ok | empty | unauthenticated | forbidden | invalid | unavailable`). `Code.Unauthenticated` redirects the whole page to login, `Code.PermissionDenied` shows the existing 403 `Result`, `Code.InvalidArgument` shows an inline "tham số không hợp lệ" in that panel only, anything else is `unavailable`. `empty` is an `ok` response with zero rows, distinct from `unavailable` so a failure never reads as "no activity".
- No arithmetic beyond formatting: rate, CI, cohort shares and conversion arrive computed. The only derived figures are the AI-surface accepted and dismissed share of shown, which are display ratios of two returned counts and are a dash when `shown` is 0. This keeps with "frontend holds no business logic" because no new metric is defined, and keeps the report RPC unchanged.
- `model_version` is opaque text. The initiative's format `<generation>/<tier>/<ranker>/<arm>` is shown verbatim in a monospace cell, so the panel works for old rows (`serving-fallback`, bare generation ids) and for the arm `unassigned` rows the placement report may carry.
- Flag input is a free text key validated against `^[a-z0-9][a-z0-9_.-]{0,63}$` before any call. An empty flag shows the prompt state and makes no call. This avoids a dependency on a Flipt list API, which the contract does not provide.

## Risks

- The exact placement id and flag key for the first experiment are not known to this slice; the panel does not hard-code any (see OPEN_ISSUES in the slice hand-off).
- The report RPCs may legitimately take up to 2 s on 26 weeks; the 5 s timeout leaves headroom and the retention panel defaults to 12 weeks.
