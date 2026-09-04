/**
 * Thin HTTP edge (Backend-for-Frontend) — stands in for the future Next.js
 * web/wap SSR + BFF layer (team-frontend).
 *
 * It proves the wiring shape:  browser / native app  →  THIS edge (HTTP)  →
 * Gateway (gRPC, via src/client.ts)  →  backend services.
 *
 * Deliberately a plain Node http server (no framework) — a seed, not an app.
 * When this becomes team-frontend, Next.js route handlers / server actions
 * replace this file and call the same client factory in src/client.ts.
 *
 * Routes:
 *   GET /healthz        → liveness, no downstream calls
 *   GET /demo/search?q= → demo BFF route; calls the Gateway and returns JSON
 */

import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { config } from "./config.js";
import { newRequestId, REQUEST_ID_HEADER } from "./auth.js";
// Requires `npm run proto` — see src/client.ts. Uncomment alongside the client:
// import { searchListings } from "./client.js";

function sendJson(
  res: ServerResponse,
  status: number,
  body: unknown,
  requestId: string,
): void {
  const payload = JSON.stringify(body);
  res.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
    [REQUEST_ID_HEADER]: requestId,
  });
  res.end(payload);
}

async function handle(req: IncomingMessage, res: ServerResponse): Promise<void> {
  // Bridge an inbound correlation id or mint one (ADR-0004). A real BFF would
  // thread this into the Gateway call's headers for end-to-end tracing.
  const requestId =
    (req.headers[REQUEST_ID_HEADER] as string | undefined)?.trim() || newRequestId();

  const url = new URL(req.url ?? "/", `http://${req.headers.host ?? "localhost"}`);

  if (req.method === "GET" && url.pathname === "/healthz") {
    sendJson(res, 200, { status: "ok", service: config.OTEL_SERVICE_NAME }, requestId);
    return;
  }

  if (req.method === "GET" && url.pathname === "/demo/search") {
    const query = url.searchParams.get("q") ?? "";
    // ── With generated code (`npm run proto`), this route calls the Gateway ──
    // try {
    //   const hits = await searchListings(query);
    //   sendJson(res, 200, { query, hits }, requestId);
    // } catch (err) {
    //   sendJson(res, 502, { error: "gateway_unavailable", detail: String(err) }, requestId);
    // }
    // return;
    sendJson(
      res,
      501,
      {
        error: "not_generated",
        message:
          "Demo route is wired but the Connect client is not generated yet. " +
          "Run `npm run proto`, then uncomment searchListings in src/client.ts and src/server.ts.",
        query,
        gateway: config.GATEWAY_URL,
      },
      requestId,
    );
    return;
  }

  sendJson(res, 404, { error: "not_found", path: url.pathname }, requestId);
}

const server = createServer((req, res) => {
  handle(req, res).catch((err) => {
    const requestId = newRequestId();
    sendJson(res, 500, { error: "internal", detail: String(err) }, requestId);
  });
});

server.listen(config.PORT, () => {
  // eslint-disable-next-line no-console
  console.log(
    `[${config.OTEL_SERVICE_NAME}] edge listening on :${config.PORT} → gateway ${config.GATEWAY_URL}`,
  );
});
