## 1. Platform

- [x] 1.1 Alert (tones, title, description, action, closable) and Toast tone, duration, label, dismissLabel, position; tokens, tests, states, typecheck, changeset, catalog, vendor; verify `npm run check` in `fe-platform`

## 2. Facades

- [x] 2.1 Rewrite `Alert.tsx` and `ToastProvider.tsx` as facades, delete `AlertClient.tsx` and `alertView.tsx`; verify the facade tests pass

## 3. Call sites

- [x] 3.1 Wrap the Alert call sites that passed `className` in a div; scan the tokens class maps in Tailwind content; verify `tsc` has no errors
- [x] 3.2 Update the toast test that assumed a single status node; verify the full vitest run is green

## 4. Verification

- [x] 4.1 `npm run check` and `npm run build` in `team-frontend`
- [x] 4.2 Check platform-e2e alert and toast selectors (role, text) against the new markup
- [x] 4.3 `openspec validate team-frontend-feedback --strict`
