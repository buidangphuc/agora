## Decisions

- Roles follow the current team-frontend Alert exactly: only `error` (platform `danger`) is role=alert; `info`, `success` and `warning` render an `<output>` (role=status). The facade tests and platform-e2e (`get_by_role("alert")` for errors, `get_by_role("status")` for success toasts) rely on this.
- Toast live regions: the role sits on an inner element of each Radix toast (`div role=alert` for danger, `output` otherwise), not on the `li`, so the viewport list stays valid. That element carries `data-radix-toast-announce-exclude` so Radix's own visually hidden announcer does not repeat the message; without it a success toast's text would match two role=status nodes and Playwright strict locators (`get_by_role("status").filter(has_text=...)`) would fail for the first second.
- `duration: 0` maps to `Infinity` because Radix treats 0 as "use the provider default".
- `position="top"` keeps the app's toasts top right as before; the platform default stays bottom.
- Platform Alert splits a server-safe body (`alertBody.tsx`) from a client `AlertClosable`, matching the old Alert/AlertClient split; only `Alert` is exported.
- Tone colours reuse existing semantic colours (`border-text-success`, `border-text-warning`, `border-border-danger`); no new semantic tokens.
- `team-frontend` Tailwind content now includes `node_modules/@fe-platform/tokens/src/tokens.mjs`; before this, platform classes absent from team source (toast `top-4 right-4`, alert tone borders, and earlier button/checkbox classes) were never generated.

## Risks

- The Radix announcer still renders a short-lived role=status node holding only the provider label ("Thông báo"); unfiltered `get_by_role("status").first` locators could pick it, but it is visible and carries no message text.
- The task brief said role=alert for warning; the current Alert and its tests use role=status for warning, which is kept.
- The Tailwind content line is shared with other C3 branches and may need a trivial merge.
