## 1. Layout

- [x] 1.1 Add `AuthPanel` and `AuthSwitch` in `features/auth`; render `/login` and `/register` through `AuthPanel`; verify `tsc` has no errors
- [x] 1.2 Move the forms out of the inner card and make the switch link a 44 px secondary button link; verify the auth and route vitest files pass

## 2. Verification

- [x] 2.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 2.2 Load `/login` and `/register` at 375, 768 and 1280 px: no horizontal overflow, interactive controls at least 44 px high
- [x] 2.3 `openspec validate team-frontend-layout-auth --strict`
