# Tasks

> Depends on `add-flipt-infra` (Flipt must be running/reachable). The two code tracks below
> touch disjoint file sets and can run in parallel with the e2e track.
>
> Implementation note (scope): this pass wired **team-order** + **team-frontend** only.
> `docker-compose.services.yaml` and `platform-gitops` were intentionally left untouched
> (out of scope for this pass) and are noted as deferred below.

## 1. Code — team-order (Go: enforcement point + reusable bootstrap init)
- [x] `internal/featureflags/featureflags.go` + `provider.go` (new): OpenFeature client
      (`open-feature/go-sdk`) behind a tiny `Evaluator` interface, backed by the **Flipt
      provider**; `New()`/`Disabled()`/`Close()`. Flipt provider construction is isolated in
      `provider.go`. **Apply-time risk**: the exact Flipt provider package
      (`go.flipt.io/flipt-openfeature-provider`, pinned in go.mod) and its streaming /
      in-process (in-memory snapshot) eval config must be confirmed at apply time — the
      *behavior* (in-memory, streamed, ~0ms lookup) is what's required; swap only `provider.go`.
- [x] `internal/config/config.go`: added `FeatureFlags` group — `FLIPT_ADDR`,
      `FEATURE_FLAGS_ENABLED`, `FEATURE_FLAGS_EVAL_TIMEOUT_MS` with `env:`/`default:` tags;
      updated `.env.example`.
- [x] `internal/bootstrap/resources.go`: open the featureflags client in `InitResources`
      (stored on `Resources.Flags` beside `Pool`), close it in `CloseResources`. On
      disabled/unreachable it degrades to `featureflags.Disabled()` (fail-open) — never blocks boot.
- [x] `internal/handler/order.go` (`CreateOrder`): evaluates `checkout-enabled` (default
      `true`); when **false**, returns `status.Error(codes.FailedPrecondition, "checkout is
      temporarily unavailable")` before running the saga. Wired via backward-compatible
      `handler.WithFeatureFlags(...)` option (existing call sites unchanged).
- [x] Unit tests (black-box): `internal/featureflags/featureflags_test.go` uses OpenFeature's
      in-memory (fake) provider — flag on → allowed, off → blocked, provider-error → fail-open,
      disabled → default. `internal/handler/order_test.go` adds kill-switch on/off/no-flags cases.
- [ ] `docker-compose.services.yaml` (team-order env: `FLIPT_ADDR=flipt:9000`,
      `FEATURE_FLAGS_ENABLED=true`). **Deferred — out of scope for this pass** (compose file
      not edited). Local default is `localhost:9000` via config/`.env.example`.
- [ ] `go build ./... && go vet ./... && go test ./...` clean. **Deferred to Docker/CI** — Go
      is not installed on host and generated proto is gitignored, so team-order cannot compile
      locally. Code was written against real symbols and self-reviewed but NOT compiled here.
      **CI action required**: the Dockerfile runs `go mod download` then `go build`; because new
      deps were added, run `go mod tidy` (to populate `go.sum`) before the build.

## 2. Code — team-frontend (TS: server-side UX gate)  ✅ verified green locally
- [x] `src/lib/flags/index.ts` (new, `server-only`): `@openfeature/server-sdk` client with the
      Flipt provider; `getFlag("checkout-enabled", true)` + `isCheckoutEnabled()` helpers for
      Server Components / Server Actions. Fails open on any error. Never imported client-side.
- [x] `src/lib/flags/config.ts` (new): reads `FLIPT_ADDR` (+ `FEATURE_FLAGS_ENABLED`) from env,
      normalized to the Flipt REST/HTTP URL the JS provider uses. `src/lib/flags/provider.ts`
      isolates Flipt provider registration (apply-time risk noted inline).
- [x] Gated the checkout entry point server-side: `src/app/cart/page.tsx` evaluates the flag and
      passes `checkoutEnabled` to `CartView` (hides/disables the "Mua Hàng" CTA + shows a
      "Thanh toán tạm thời không khả dụng" notice when off); `src/app/checkout/page.tsx` renders
      the unavailable notice instead of `CheckoutView` on direct navigation. Browser only ever
      receives the resolved boolean.
- [x] Vitest test: `src/lib/flags/index.test.ts` (in-memory provider) — on → true, off → false,
      unavailable → fail-open true. Matches the frontend-quality tier.
- [x] `npm run check` (biome + `tsc --noEmit` + `vitest run`) **GREEN — exit 0, 123 tests pass**
      (3 new flag tests). `next build` not run locally (not part of `check`; deferred to CI).
- [ ] `docker-compose.services.yaml` (team-frontend env: `FLIPT_ADDR=flipt:8080`).
      **Deferred — out of scope for this pass**. NOTE: the JS provider uses Flipt's REST/HTTP
      endpoint (:8080), unlike Go which uses gRPC (:9000).

## 3. Code — platform-gitops (runtime endpoint wiring)
- [ ] `argocd/apps/team-order.yaml` / `team-frontend.yaml` Helm `values.env` (`FLIPT_ADDR`,
      `FEATURE_FLAGS_ENABLED`). **Deferred — out of scope** (platform-gitops is a separate repo,
      not touched in this pass).
- [ ] ArgoCD diff / sync verification. **Deferred** (depends on the gitops edit above).

## 4. E2E (platform-e2e)
- [x] `team-order/FEATURES.yaml`: added `order.checkout-kill-switch` mapping 1:1 to the spec
      scenarios; **status `planned`** (flips to `automated` when the platform-e2e scenario is green).
- [x] platform-e2e `.feature` + steps + page objects (Flipt-API seeding of `checkout-enabled`;
      OFF → checkout hidden/blocked + `CreateOrder` rejected; ON → checkout completes). AUTHORED:
      `tests/e2e/features/feature_flags/checkout_kill_switch.feature` (5 scenarios, names mapped 1:1
      to the spec `#### Scenario:` titles — the two pure backend-lifecycle scenarios "flag client
      shuts down cleanly" is covered by team-order Go unit tests, not e2e; "service starts with a
      working flag client" IS covered as an observable ON-path order placement) +
      `step_definitions/feature_flags_steps.py` + `step_definitions/test_checkout_kill_switch.py`
      (binder). New plumbing: `tests/e2e/flows/flipt_flow.py::set_checkout_flag` (toggles the flag via
      Flipt REST at `FLIPT_URL`) and a `FLIPT_URL` setting; UI gating asserted via new
      `CartPage.checkout_unavailable_notice` / `CheckoutPage.checkout_unavailable_notice` locators.
      Reused `buyer_steps` + `login_via_api` + `CartService`/`OrderService`. Wired into `make collect`
      (all 5 scenarios collect); `ruff`/`black` clean.
- [ ] Run green against the local stack (`make -C platform-e2e features-check` / `spec-check`) + flip
      `order.checkout-kill-switch` to `automated` (set `covered_by`). — GATED ON CI/STACK: needs Flipt
      running + a compiled team-order + team-frontend (Go not buildable on this host). Entry stays
      `status: planned` (no `covered_by`, previous invalid `covered_by` removed so
      `scripts/features.py` passes).

## 5. Archive
- [ ] Kill-switch proven end-to-end (one Flipt toggle flips both UI entry and the team-order RPC,
      no redeploy; fail-open verified). **Deferred** — pending Go build in CI + platform-e2e run.
- [ ] `make -C platform-e2e spec-check CHANGE=wire-openfeature` + `pytest` + lint green. **Deferred**.
- [ ] Archive (`/opsx:archive wire-openfeature`). **Deferred**.
