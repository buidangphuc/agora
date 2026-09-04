# _service-ts — Edge / BFF seed (Connect-ES)

A **seed** (skeleton, not a full app) for the platform's **edge**: a
Backend-for-Frontend that consumes platform gRPC contracts via **Connect-ES** and
exposes a thin HTTP surface. It later becomes **`team-frontend`** — a Next.js
web/wap SSR + BFF app.

## Role: this is the EDGE, not a backend service

Unlike the Go/Python/JVM service templates, this one is **not** an owned-DB
backend. It is the consumer at the front of the platform:

```
browser / native app  →  THIS edge (HTTP/SSR)  →  Gateway (gRPC)  →  backend services
```

- **No Postgres, no migrations.** The edge owns no domain data; it fetches
  everything through the Gateway.
- **ARCHITECTURE Rule 1:** the frontend/edge calls **only the Gateway**, never a
  backend service directly. `src/client.ts` points every client at `GATEWAY_URL`.
- This seed proves the **Connect-ES → Gateway** wiring shape; the real UI arrives
  when it becomes `team-frontend` (Next.js).

## Layout

| Path | Purpose |
| --- | --- |
| `src/config.ts` | Env read + validation (hand-rolled, no zod). |
| `src/auth.ts` | Bearer + `x-request-id`/`traceparent` propagation as a Connect interceptor (ADR-0003/0004). |
| `src/client.ts` | Connect client factory pointed at the Gateway (gRPC transport). |
| `src/server.ts` | Thin HTTP/BFF surface: `GET /healthz`, `GET /demo/search`. |
| `tests/auth.test.ts` | Unit tests for the pure helpers — green **before** codegen. |
| `buf.gen.yaml` | Connect-ES codegen → `./generated` (gitignored). |
| `proto-vendor/` | Where platform-core's proto module is vendored (submodule, pinned). |

## Instantiate this seed

1. **Rename.** `package.json` `name` is a placeholder (`team-frontend-edge`);
   rename it and this folder to your real service/repo name. Update
   `OTEL_SERVICE_NAME` in `.env` to match.
2. **Install.** `npm install`.
3. **Vendor the proto module.** Follow `proto-vendor/README.md` — add the
   platform-core proto module as a git submodule pinned at a tag (placeholder
   `proto/v0.1.0`).
4. **Generate the client.** `npm run proto` → writes `./generated/` (gitignored).
5. **Wire the generated clients.** Uncomment the generated-client block in
   `src/client.ts` and the demo call in `src/server.ts`.
6. **Run.** `cp .env.example .env`, then `npm run dev`.
   - `curl localhost:3000/healthz` → `{"status":"ok",...}`
   - `curl "localhost:3000/demo/search?q=house"` → Gateway result (once wired).

## Scripts

| Script | Does |
| --- | --- |
| `npm run proto` | `buf generate` → `./generated` (needs `proto-vendor/`). |
| `npm run dev` | Run the edge with `--watch` (Node type-stripping). |
| `npm run build` | `tsc` → `dist/`. |
| `npm run check` | `biome check` + `tsc --noEmit` + `vitest run` (CI gate). |
| `npm test` | `vitest run`. |

## Rules of the road

- **Call only the Gateway** (Rule 1). Do not add clients to backend services
  directly; add Gateway RPCs.
- **Don't edit `generated/`.** It is machine-output, gitignored, and regenerated
  by `npm run proto`. Change contracts in platform-core's proto, cut a new tag,
  bump the pin (see `proto-vendor/README.md`).
- **Auth/trace at the edge** (ADR-0003/0004): the edge attaches the service
  bearer and propagates `x-request-id` + `traceparent` on the Gateway hop. Real
  user identity (JWT/cookie session) is resolved here later, once the Gateway
  defines it — services stay mechanism-agnostic.
