# team-frontend

Next.js 14 (App Router, SSR, `output: "standalone"`) web UI for the marketplace. It is the **UI shell**: it renders pages, shapes data for display and holds the `httpOnly` session cookie. It owns no business capability, no database and no Kafka/RabbitMQ topics. Each feature's logic lives in the service that owns the capability (identity, listing, order, search, ...).

Status: deployed (compose service `team-frontend`, port `:3000`). Stack: React 18, Tailwind 3, Connect-ES (`@connectrpc/connect-node`), OpenFeature + Flipt, Biome, Vitest.

## 1. Contract

**Serves** (no gRPC/Connect server; HTTP only):

| Surface | What | Authorization |
|---|---|---|
| 37 pages under `src/app/(shop)` and `src/app/(checkout)` | Buyer, account, seller, admin, chat, assistant, checkout (route groups below) | Per page/layout, from the gateway scopes in the session JWT |
| `GET /api/suggest?q=` | Search type-ahead, proxies `suggest` | Public; errors return empty suggestions |
| `GET /api/listings?status=&cursor=` | "Load more" for the consumer feed | Public; errors return an empty page |
| `POST /api/assistant/stream` | Relays `ChatService.StreamChat` deltas as a plain-text stream | Session cookie forwarded, no local check; the gateway decides. 400 on empty `message` |
| `GET /api/admin/metrics` | Cockpit refresh, proxies `fetchCockpit` | 401 without a session; the gateway enforces scope `admin` (401/403 passed through) |

Route groups and page-level gates:

| Group | Routes | Gate |
|---|---|---|
| `(shop)` public | `/`, `/search`, `/listing/[id]`, `/shop/[id]`, `/vouchers`, `/s/[code]` (share-link redirect), `/login`, `/register`, `/sell`, `/cart`, `/assistant` | no page-level redirect (the gateway still scopes the data) |
| `(shop)` signed-in | `/favorites`, `/chat`, `/notifications`, `/account/{orders,orders/[id],addresses,following,referral,security,verification}` | redirect to `/login` without a session |
| `(shop)` | `/chat/[id]` | redirects to `/chat?thread=<id>` |
| `(shop)/seller/*` | dashboard, `new`, `[id]/edit`, `orders`, `orders/[id]`, `analytics`, `ads`, `bundles`, `plans`, `shop`, `wallet` | `seller/layout.tsx`: `/login` if anonymous; "seller account needed" result unless scope `listing.write` |
| `(shop)/admin/cockpit` | Ops cockpit | `/login` if anonymous, denied view unless scope `admin`; the gateway re-enforces it on the data |
| `(shop)/dev/ui` | UI component gallery | `notFound()` when `NODE_ENV=production` |
| `(checkout)` | `/checkout`, `/checkout/pay/[id]` | `/checkout` redirects to `/login` if anonymous and to `/cart` if the cart is empty |

**Consumes**: only `team-gateway` (`GATEWAY_URL`), through `@connectrpc/connect-node` clients over HTTP/1.1. All RPC code lives in `src/lib/gateway/*` (`server-only`); pages and `src/features/*` import from there. Clients are built per request with the caller's token (`makeClients(token)`), so the bearer is never a process singleton and never reaches the browser. Public reads retry once without the bearer (`anonymousFallback`) so a stale session degrades to anonymous browsing. `src/middleware.ts` removes an expired or malformed `session` cookie before render.

Generated services in use (`src/generated/platform/*`, from the vendored `proto/platform/*`): search, listing, identity (Auth, Address), engagement, order (Cart, Order), payment, chat, ai, recommendation, promotion (Voucher, FlashSale, Subscription, Sponsored), notification, plus referral, sharing, verification, follow and cockpit/analytics helpers in their own `src/lib/gateway/*.ts` modules. The gateway forwards each RPC to the owning service.

Login/register forward the browser IP and user agent to the gateway (`src/lib/gateway/client-context.ts`): the `X-Forwarded-For` entry `TRUSTED_PROXY_HOPS` places from the right, never the client-controlled leftmost one. The gateway trusts it only from this server (`TRUSTED_PROXIES=team-frontend-svc` in compose).

Session: login stores the JWT in cookie `session` (`httpOnly`, `sameSite=lax`, `path=/`, `maxAge=3600`; `src/features/auth/actions.ts`). The server attaches it as `Authorization: Bearer`.

## 2. Events

None produced or consumed over a broker. The browser sends a best-effort tracking beacon batch directly to the **gateway** `POST {NEXT_PUBLIC_GATEWAY_URL}/api/track` (`src/lib/analytics/queue.ts`: `sendBeacon`, else `fetch` with `keepalive` and `credentials: include`; batches of 20 or every 2 s). This is the one browser-to-gateway call. An optional GTM dataLayer destination loads only if `NEXT_PUBLIC_GTM_ID` is set at build time.

## 3. Data

No database, no migrations. State is the session cookie plus whatever the gateway returns.

## 4. Configuration

Read by the code (see `.env.example`; there is no drift gate for this repo):

| Var | Default | Used in |
|---|---|---|
| `GATEWAY_URL` | `http://127.0.0.1:8080` (must be http/https or startup throws) | `src/lib/gateway/config.ts` |
| `NEXT_PUBLIC_GATEWAY_URL` | `http://localhost:8080` (browser-visible, inlined at build) | `src/lib/analytics/queue.ts` |
| `OTEL_SERVICE_NAME` | `team-frontend` | `src/lib/gateway/config.ts` |
| `FEATURE_FLAGS_ENABLED` | `true` (anything but `false` enables) | `src/lib/flags/config.ts` |
| `FLIPT_ADDR` | `flipt:8080` (`http://` prepended if missing; Flipt REST port) | `src/lib/flags/config.ts` |
| `TRUSTED_PROXY_HOPS` | `1` (values below 1 or non-numeric fall back to 1) | `src/lib/gateway/client-context.ts` |
| `NEXT_PUBLIC_GTM_ID` | empty, GTM disabled (Dockerfile build `ARG`, inlined at build) | `src/lib/analytics/destinations/gtm.ts` |
| `NEXT_PUBLIC_MEDIA_BASE_URL` | `http://localhost:9000/listing-images` | `src/lib/media.ts` |

`NEXT_PUBLIC_*` values are baked in at `next build`; changing them at runtime has no effect. `NEXT_PUBLIC_MEDIA_BASE_URL` is not in `.env.example`. Feature flags are evaluated server-side only.

## 5. Run locally

Root compose (from the repo root; service `team-frontend`, container `team-frontend-svc`, `:3000`; it needs the gateway at `:8080`, which has no `depends_on` link, so start it too):

```bash
docker compose -f docker-compose.services.yaml up --build team-frontend team-gateway   # plus the services you need
```

Standalone dev (needs a gateway on `GATEWAY_URL`):

```bash
cd team-frontend
npm ci
npm run proto        # buf generate, required before dev/build/tsc/test
npm run dev          # http://localhost:3000
```

## 6. Build, test and lint

| Command | Does |
|---|---|
| `npm run proto` | `buf generate` from `proto/` into `src/generated/` |
| `npm run dev` / `build` / `start` | Next dev server / production build / serve the build |
| `npm run typecheck` | `tsc --noEmit` |
| `npm test` | `vitest run` (jsdom, `vitest.setup.ts`) |
| `npm run check` | **PR gate**: `biome check .` + `tsc --noEmit` + `node scripts/check-tokens.mjs` + `vitest run` |

`UI_SYSTEM_DESIGN.md` section 7 requires `npm run check` on every PR touching this repo. Node >= 20 (Dockerfile uses node 22).

### UI system

Three tiers, documented in `platform-core/docs/UI_SYSTEM_DESIGN.md`: primitives in `tailwind.config.ts` (primary `#ee4d2d`), semantic aliases (`bg-action-primary`, `text-text-primary`, ...) in `tailwind.config.ts` backed by CSS variables in `src/app/globals.css`, and components in `src/components/ui/`. `scripts/check-tokens.mjs` fails on raw hex, `rgb()/hsl()` literals, arbitrary Tailwind values (`text-[9px]`) and literal inline-style colours in `src/**/*.tsx` (excluding `src/generated`); exempt a line with a `tokens-allow: <reason>` comment. Browse components at `/dev/ui` (dev only). Shell chrome (header, disclaimer banner, floating chat) is `src/components/shell/ConsumerShell.tsx`; `FloatingChatBubble` and `ToastProvider` are in `src/components/ui/`.

## 7. Spec and verification

- Feature manifest: `FEATURES.yaml` (94 features: 85 `automated`, 9 `planned`). The frontend owns only presentational and cross-cutting journeys; capability journeys live in the owning repo's manifest.
- E2E: `platform-e2e` (pytest-bdd + Playwright) drives `team-frontend:3000` through the gateway `:8080`. Gates: `make -C platform-e2e features-check` and `make -C platform-e2e spec-check CHANGE=<id>`.
- Changes are specified through OpenSpec (`openspec/changes/<id>`), then `FEATURES.yaml`, then platform-e2e, then `npm run check`, per the root README's ASDLC.

## 8. Gotchas

- `src/generated/` is gitignored; run `npm run proto` after clone and after every contract change. Never hand-edit it.
- `proto/` is vendored from `platform-core`; never edit it here (ADR-0001).
- `src/lib/gateway/*` is `server-only`; do not import it from client components.
- The gateway answers Unauthenticated to any invalid bearer, even on public RPCs; use `anonymousFallback` for public reads.
- `NEXT_PUBLIC_*` are build-time. In the e2e stack `NEXT_PUBLIC_GTM_ID` is set as a build arg so `window.dataLayer` initialises.
- UI copy is mostly Vietnamese, including the disclaimer banner in `ConsumerShell.tsx`.

## 9. Known gaps

- Session cookie is set without `secure` (`src/features/auth/actions.ts`), so it is sent over plain HTTP if the site is served that way.
- `/api/suggest` and `/api/listings` swallow every error and return empty results, hiding gateway outages from the UI.
- `.env.example` omits `NEXT_PUBLIC_MEDIA_BASE_URL`, whose default points at a local MinIO URL that is wrong outside local dev.
- `FEATURES.yaml` has 9 `planned` features with no automated coverage yet.

## 10. Links

- Rules: root `AGENTS.md` (Rule 1: frontend talks only to the gateway; port table `:3000`).
- `platform-core/docs/UI_SYSTEM_DESIGN.md`, `platform-core/docs/ARCHITECTURE.md`.
- ADRs: `platform-core/docs/ADR/0001-proto-distribution.md`, `0003-auth-model.md`, `0006-rs256-jwks-auth.md`.
