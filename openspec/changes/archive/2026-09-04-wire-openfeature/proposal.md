## Why

`add-flipt-infra` stands up a Flipt server but nothing evaluates a flag yet. This change
wires the **CNCF OpenFeature SDK** (vendor-neutral) with the **Flipt provider** into the
Go services and into `team-frontend`, and proves it end-to-end with **one representative
flag**: an emergency **kill-switch on checkout**.

The point is to make **release** a runtime action, separate from **deployment**. Checkout
(`team-order`'s purchase saga) is already deployed. With this flag an on-call engineer can
turn checkout **off in seconds from the Flipt UI** during an incident (e.g. payment
provider melting down) — no PR, no rebuild, no pod restart — and turn it back **on** the
same way. The flag is evaluated **in-process against an in-memory snapshot** that Flipt
keeps fresh over a gRPC stream, so the check on the hot path costs ≈0ms.

OpenFeature (not Flipt's own client) is the abstraction at every call site, so Flipt stays
a swappable provider and we avoid vendor lock-in.

## What Changes

This is a **cross-cutting pattern** described once and applied to two representative repos
with **disjoint file sets**. The reusable shape:

> Add a small `featureflags` init that constructs an OpenFeature client backed by the
> Flipt provider (streaming → in-memory eval), owned by the service's existing
> bootstrap/lifecycle; read the client where a decision is made; default-value semantics
> make the flag system's own outage a non-event.

- **team-order** (Go — enforcement point, owns the checkout saga; Rule 3):
  - New `internal/featureflags` package: build the OpenFeature client with the Flipt
    provider (streaming in-process evaluation), wired into `internal/bootstrap` Resources
    (open on startup, close on shutdown) next to the pgx pool.
  - Evaluate `checkout-enabled` in the `CreateOrder` handler; when the flag is **off**,
    reject with a clear gRPC status (`FAILED_PRECONDITION`/`UNAVAILABLE`) instead of
    running the saga. Default value **true** (fail-open) so a Flipt outage never blocks checkout.
  - `internal/config`: add `FLIPT_ADDR` (+ enable/timeout) with `env:`/`default:` tags.
  - `docker-compose.services.yaml` + `platform-gitops` `argocd/apps/team-order.yaml` values:
    add `FLIPT_ADDR=flipt:9000` (compose) / `flipt.marketplace.svc:9000` (cluster).
- **team-frontend** (TS — UX layer; Rule 1, server-side only):
  - New server-only `src/lib/flags` module: an `@openfeature/server-sdk` client with the
    Flipt provider, evaluated **only** in Server Components / Server Actions (never shipped
    to the browser).
  - Gate the checkout **entry point** (the "Place order" / checkout button in the cart /
    `CheckoutView`) on `checkout-enabled`: when off, hide/disable it and show a short
    "checkout temporarily unavailable" notice.
  - `GATEWAY_URL`-style config: add `FLIPT_ADDR` env in `docker-compose.services.yaml` +
    `argocd/apps/team-frontend.yaml` values.
- **The same `featureflags` bootstrap shape is reusable** by the other Go services
  (`team-domain`, `team-gateway`, …) when they adopt flags — but this change **proves it on
  `team-order` + `team-frontend` only**, with a single flag.
- **E2E (platform-e2e)** — a user-facing kill-switch scenario driven through the gateway/UI
  (see the e2e track in tasks.md), owned by `team-order/FEATURES.yaml` (checkout is
  team-order's capability).

**No proto change.** Flag evaluation is infra/config, not a marketplace RPC — nothing in
`platform-core/packages/proto` changes.

## Non-goals

- **No migrating existing env/config flags** (`*_ENABLED` toggles, feature envs) into Flipt —
  only the new `checkout-enabled` kill-switch is introduced.
- **No percentage rollouts, targeting rules, experiments, or A/B analytics** — the flag is a
  plain boolean kill-switch (on/off).
- **No Statsig** or any external experimentation/analytics platform (a separate, later decision).
- **No second flag and no rollout to every service** — the pattern is documented for reuse but
  only `team-order` + `team-frontend` are wired, gating one feature.
- **No gateway business logic** — the gateway keeps forwarding only; the kill-switch is
  decided in the owning service (`team-order`) and reflected in the UI (`team-frontend`),
  not in `team-gateway` (Rule 2).
- **No new proto/contract**.
