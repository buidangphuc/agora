import { Code, ConnectError } from "@connectrpc/connect";
import { describe, expect, it } from "vitest";

import { createGatewayClients } from "./client";
import { loadGatewayConfig } from "./config";
import { ApiError, toApiError } from "./errors";

interface Seen {
  url: string;
  headers: Headers;
}

function fakeFetch(respond: (seen: Seen, call: number) => Response): {
  fetch: typeof globalThis.fetch;
  seen: Seen[];
} {
  const seen: Seen[] = [];
  const fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const entry = {
      url: String(input),
      headers: new Headers(init?.headers),
    };
    seen.push(entry);
    return respond(entry, seen.length);
  }) as typeof globalThis.fetch;
  return { fetch, seen };
}

const ok = () =>
  new Response(JSON.stringify({ listing: { id: "l1", title: "Lamp" } }), {
    status: 200,
    headers: { "content-type": "application/json" },
  });

const unauthenticated = () =>
  new Response(
    JSON.stringify({ code: "unauthenticated", message: "expired" }),
    {
      status: 401,
      headers: { "content-type": "application/json" },
    },
  );

describe("toApiError", () => {
  const cases: [Code, string][] = [
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.NotFound, "not_found"],
    [Code.InvalidArgument, "invalid"],
    [Code.FailedPrecondition, "invalid"],
    [Code.OutOfRange, "invalid"],
    [Code.AlreadyExists, "invalid"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "unavailable"],
    [Code.ResourceExhausted, "unavailable"],
    [Code.Internal, "internal"],
    [Code.Unknown, "internal"],
  ];
  it.each(cases)("maps Connect code %s to %s", (code, kind) => {
    const err = toApiError(new ConnectError("boom", code), "req-1");
    expect(err).toBeInstanceOf(ApiError);
    expect(err.kind).toBe(kind);
    expect(err.code).toBe(code);
    expect(err.message).toBe("boom");
    expect(err.requestId).toBe("req-1");
  });

  it("maps a non-Connect error to internal", () => {
    expect(toApiError(new TypeError("x")).kind).toBe("internal");
    expect(toApiError("nope").kind).toBe("internal");
  });

  it("returns an ApiError unchanged", () => {
    const e = new ApiError("not_found", "gone");
    expect(toApiError(e)).toBe(e);
  });
});

describe("gateway client", () => {
  it("sends bearer and a request id", async () => {
    const { fetch, seen } = fakeFetch(ok);
    const { listing } = createGatewayClients({
      token: " tok ",
      requestId: "rid-9",
      baseUrl: "http://gw.test",
      fetch,
    });
    const res = await listing.getListing({ id: "l1" });
    expect(res.listing?.title).toBe("Lamp");
    expect(seen[0].url).toBe(
      "http://gw.test/platform.listing.v1.ListingService/GetListing",
    );
    expect(seen[0].headers.get("authorization")).toBe("bearer tok");
    expect(seen[0].headers.get("x-request-id")).toBe("rid-9");
  });

  it("generates a request id when none is given and omits an absent token", async () => {
    const { fetch, seen } = fakeFetch(ok);
    const { listing } = createGatewayClients({
      baseUrl: "http://gw.test",
      fetch,
    });
    await listing.getListing({ id: "l1" });
    expect(seen[0].headers.get("authorization")).toBeNull();
    expect(seen[0].headers.get("x-request-id")).toMatch(/^[0-9a-f-]{36}$/);
  });

  it("retries once without the bearer on Unauthenticated when fallback is on", async () => {
    const { fetch, seen } = fakeFetch((_s, call) =>
      call === 1 ? unauthenticated() : ok(),
    );
    const { listing } = createGatewayClients({
      token: "stale",
      anonymousFallback: true,
      baseUrl: "http://gw.test",
      fetch,
    });
    const res = await listing.getListing({ id: "l1" });
    expect(res.listing?.id).toBe("l1");
    expect(seen).toHaveLength(2);
    expect(seen[0].headers.get("authorization")).toBe("bearer stale");
    expect(seen[1].headers.get("authorization")).toBeNull();
  });

  it("surfaces Unauthenticated when fallback is off", async () => {
    const { fetch, seen } = fakeFetch(unauthenticated);
    const { listing } = createGatewayClients({
      token: "stale",
      baseUrl: "http://gw.test",
      fetch,
    });
    await expect(listing.getListing({ id: "l1" })).rejects.toSatisfy(
      (e) => toApiError(e).kind === "unauthenticated",
    );
    expect(seen).toHaveLength(1);
  });
});

describe("loadGatewayConfig", () => {
  it("defaults to the local gateway", () => {
    expect(loadGatewayConfig({}).gatewayUrl).toBe("http://127.0.0.1:8080");
  });
  it("accepts an http(s) override", () => {
    expect(
      loadGatewayConfig({ GATEWAY_URL: "https://gw.example" }).gatewayUrl,
    ).toBe("https://gw.example");
  });
  it("rejects a non-http URL", () => {
    expect(() => loadGatewayConfig({ GATEWAY_URL: "ftp://x" })).toThrow(
      /GATEWAY_URL/,
    );
    expect(() => loadGatewayConfig({ GATEWAY_URL: "nope" })).toThrow(
      /GATEWAY_URL/,
    );
  });
});
