## Context

`add-flipt-infra` provides a running Flipt server (`flipt:9000` gRPC in compose,
`flipt.marketplace.svc:9000` in-cluster). Nothing evaluates a flag yet. AGENTS.md §9 lists
feature flags as an open seam — this is greenfield; there is no existing flag abstraction to
extend. This change introduces the evaluation layer and proves it with one kill-switch.

The governing idea: **deployment and release are different actions.**

| | Deployment (already exists) | Release (this change) |
|---|---|---|
| Mechanism | ArgoCD/GitOps ships a container | Flip a flag at runtime |
| Unit | Image + manifest | Boolean/variant value |
| Latency | Minutes (PR → build → sync → ready) | Seconds (UI toggle → stream → in-mem) |
| Who | CI + reviewer | On-call engineer |
| Reversible by | Another deploy | Flipping the flag back |

The checkout kill-switch is the smallest thing that makes that difference real and testable.

## Decisions

- **Abstraction: OpenFeature SDK, provider: Flipt.** Every call site uses the CNCF
  OpenFeature API (`open-feature/go-sdk` in Go, `@openfeature/server-sdk` in TS). Flipt is
  registered as the **provider** behind that API. Call sites never import a Flipt client
  directly, so Flipt is swappable — this is the anti-lock-in choice. (Statsig, if ever
  adopted for A/B, would be a *different provider* behind the same API — explicitly out of
  scope here.)

- **Streaming → in-memory evaluation (≈0ms checks).** The provider is configured for
  **in-process/client-side evaluation**: it pulls the flag-state snapshot from Flipt and
  keeps it in memory, re-syncing over a gRPC stream when state changes. A flag check
  (`client.BooleanValue("checkout-enabled", default, evalCtx)`) is therefore a local map
  lookup, not a per-request network round-trip — safe to put on the checkout hot path. A
  toggle in the Flipt UI propagates to the snapshot within the stream's refresh, so the
  change takes effect in seconds with **no redeploy or restart**.

- **Where the provider is initialized (Go).** In each service's existing
  bootstrap/lifecycle, alongside the pgx pool. For `team-order`: a new `internal/featureflags`
  package builds the OpenFeature client + Flipt provider in `bootstrap.InitResources`
  (stored on `Resources`), and `CloseResources` shuts the provider/stream down. This mirrors
  how `team-domain` opens Kafka/addons in `OpenResources`. The **same shape is copy-ready**
  for other Go services (`team-domain`, `team-gateway`, …) when they adopt flags — a shared,
  disjoint per-repo init, not a central library (matches the polyrepo "mirror the reference
  template" convention, AGENTS.md §7).

- **Where it is evaluated (Go).** In `team-order`'s `CreateOrder` handler — the **owning
  service** for the purchase saga (Rule 3). When `checkout-enabled` is off it returns a gRPC
  status (`FAILED_PRECONDITION`) before touching the saga. This is the authoritative
  enforcement point.

- **The gateway stays logic-free (Rule 2).** The kill-switch is **not** evaluated in
  `team-gateway`; it keeps forwarding. Putting a business kill-switch in the gateway would
  make it hold release policy for a downstream domain. So enforcement lives in `team-order`,
  and the UX reflection lives in `team-frontend`.

- **Frontend: server-side vs client-side evaluation.** The frontend evaluates the flag
  **only server-side** — in Server Components / Server Actions via a `server-only` `src/lib/flags`
  module using `@openfeature/server-sdk`. The **browser never evaluates flags** and never
  learns the Flipt endpoint; it receives already-resolved UI (button shown or hidden). This
  keeps flag state off the client and avoids a flash of the gated control.

- **Rule 1 tension, resolved deliberately.** Rule 1 says the frontend talks only to the
  gateway, never to a service directly. The frontend's server runtime reading flag state
  from Flipt is treated as a **runtime configuration/release read** — the same class as
  reading `GATEWAY_URL` or exporting to the OTel collector — **not** a business-service RPC.
  Flipt is shared infra, not a marketplace domain service; no business data or logic bypasses
  the gateway, and the browser still only ever hits the gateway. This is a narrow, explicit
  exception scoped to server-side infra reads (also noted under Risks). The authoritative
  decision is still enforced server-side in `team-order`; the frontend flag read is UX only.

- **Kill-switch semantics + fail-open default.** `checkout-enabled` is a boolean. The SDK
  **default value is `true`** (feature available). Rationale: an *emergency* kill-switch is an
  explicit human OFF action during an incident; if Flipt itself is unreachable, checkout must
  **not** go down because the flag system did — so evaluation falls back to the last in-memory
  snapshot, and to `true` if none. Turning checkout off is always a deliberate toggle, never a
  side effect of a Flipt outage. Both layers evaluate the same flag/default (defense in depth:
  UI hides the entry, service rejects the RPC).

- **Config surface.** `team-order` and `team-frontend` each read `FLIPT_ADDR` from env
  (`flipt:9000` in compose, `flipt.marketplace.svc:9000` in-cluster), added to
  `docker-compose.services.yaml` and each repo's `argocd/apps/*.yaml` Helm `values.env`. Go
  uses the reflection-based `env:`/`default:` config with a `.env.example` drift gate
  (AGENTS.md §7).

## Risks / Trade-offs

- **Frontend→Flipt is a new outbound path (Rule 1 bend).** Mitigated by: server-side only,
  never the browser; treated as config/infra read not a domain call; authoritative
  enforcement still in `team-order`. If even this is judged too much, the fallback is to
  expose evaluation through a thin gateway route — heavier, and it nudges the gateway toward
  holding a flag concern, so not chosen now.

- **Snapshot staleness window.** In-memory evaluation means a toggle takes effect only after
  the stream refreshes (seconds). Acceptable — and necessary — for a ≈0ms hot-path check; a
  kill-switch tolerates a few seconds. Documented so no one expects strict per-request freshness.

- **Fail-open could keep a broken checkout up if Flipt dies mid-incident.** Deliberate: a
  kill-switch outage should not itself cause an outage. Payment-level failures are handled by
  the saga/payment paths, not by the flag system.

- **Provider package specifics.** The exact OpenFeature Flipt provider package and its
  streaming/in-process-evaluation configuration (vs server-side per-eval) is an implementation
  detail to confirm at apply time; the design requires the *behavior* (in-memory, streamed),
  and the provider must be configured to deliver it.

- **Two evaluation points (service + UI) can diverge** if only one is toggled. Mitigated by a
  single flag key and shared default, and by the e2e asserting both the UI hide and the
  service block from one toggle.
