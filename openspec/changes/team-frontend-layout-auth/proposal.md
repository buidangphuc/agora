## Why

Plan P10 section F rebuilds the buyer page layouts on `@fe-platform/ui`. The login and register pages still used a narrow nested card with a second heading inside it and a small inline text link to switch between them, which is below the 44 px touch target and does not follow the fe-platform visual language (mobile-first, airy, large radius, indigo).

## What Changes

- New `team-frontend/src/features/auth/AuthPanel.tsx`: `AuthPanel` renders the page header (the existing `h1` plus a short description) and one padded `Card`; `AuthSwitch` renders the divided footer that links to the other auth page.
- `/login` and `/register` route pages render their form inside `AuthPanel`; the `h1` text is unchanged ("Đăng nhập", "Tạo tài khoản").
- `LoginForm` and `RegisterForm` drop the inner `Card` and the `h2` greeting (its text moves to the page description); the "Đăng ký ngay" / "Đăng nhập ngay" links become full-width secondary button links (`size="lg"`, 44 px) inside `AuthSwitch`. The unused `FormItem` import is removed.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/app/(shop)/login/page.tsx`, `team-frontend/src/app/(shop)/register/page.tsx`, `team-frontend/src/features/auth/{AuthPanel,LoginForm,RegisterForm}.tsx`.
- Field ids, labels, button and link accessible names, the `role="alert"` error and the hidden `returnUrl` input are unchanged, so unit tests and the platform-e2e page objects keep working.

## Non-goals

Server actions, validation, return-URL handling, the shared shell, `src/components/ui/*`, the shared `AuthCardSkeleton`, dark theme.
