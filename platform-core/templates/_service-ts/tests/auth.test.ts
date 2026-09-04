/**
 * Unit tests for the pure header/propagation helpers and config validation.
 *
 * These intentionally import ONLY src/auth.ts and src/config.ts — no generated
 * code, no live transport — so `vitest run` is green before `npm run proto`.
 */

import { describe, expect, it } from "vitest";
import {
  applyEdgeHeaders,
  AUTHORIZATION_HEADER,
  bearer,
  newRequestId,
  REQUEST_ID_HEADER,
  TRACEPARENT_HEADER,
} from "../src/auth.js";
import { ConfigError, loadConfig } from "../src/config.js";

describe("bearer", () => {
  it("formats a lowercase bearer credential", () => {
    expect(bearer("abc123")).toBe("bearer abc123");
  });
});

describe("newRequestId", () => {
  it("returns a unique uuid each call", () => {
    const a = newRequestId();
    const b = newRequestId();
    expect(a).not.toBe(b);
    expect(a).toMatch(/^[0-9a-f-]{36}$/i);
  });
});

describe("applyEdgeHeaders", () => {
  it("attaches authorization when a token is given", () => {
    const h = new Headers();
    applyEdgeHeaders(h, { token: "secret" });
    expect(h.get(AUTHORIZATION_HEADER)).toBe("bearer secret");
  });

  it("omits authorization when no token (or blank) is given", () => {
    const h = new Headers();
    applyEdgeHeaders(h, { token: "   " });
    expect(h.has(AUTHORIZATION_HEADER)).toBe(false);
  });

  it("forwards an existing request id unchanged", () => {
    const h = new Headers();
    const returned = applyEdgeHeaders(h, { requestId: "req-42" });
    expect(h.get(REQUEST_ID_HEADER)).toBe("req-42");
    expect(returned).toBe("req-42");
  });

  it("mints a request id when none is supplied", () => {
    const h = new Headers();
    const returned = applyEdgeHeaders(h, {});
    expect(h.get(REQUEST_ID_HEADER)).toBe(returned);
    expect(returned).toMatch(/^[0-9a-f-]{36}$/i);
  });

  it("forwards traceparent only when present", () => {
    const withTp = new Headers();
    applyEdgeHeaders(withTp, { traceparent: "00-abc-def-01" });
    expect(withTp.get(TRACEPARENT_HEADER)).toBe("00-abc-def-01");

    const withoutTp = new Headers();
    applyEdgeHeaders(withoutTp, {});
    expect(withoutTp.has(TRACEPARENT_HEADER)).toBe(false);
  });
});

describe("loadConfig", () => {
  it("applies defaults for an empty environment", () => {
    const c = loadConfig({});
    expect(c.GATEWAY_URL).toBe("http://localhost:8080");
    expect(c.PORT).toBe(3000);
    expect(c.OTEL_SERVICE_NAME).toBe("team-frontend-edge");
    expect(c.AUTH_BEARER_TOKEN).toBeUndefined();
    expect(c.OTEL_EXPORTER_OTLP_ENDPOINT).toBeUndefined();
  });

  it("reads and coerces provided values", () => {
    const c = loadConfig({
      GATEWAY_URL: "https://gw.internal:8443",
      PORT: "3100",
      AUTH_BEARER_TOKEN: "tok",
      OTEL_EXPORTER_OTLP_ENDPOINT: "http://localhost:4318",
      OTEL_SERVICE_NAME: "edge-x",
    });
    expect(c.GATEWAY_URL).toBe("https://gw.internal:8443");
    expect(c.PORT).toBe(3100);
    expect(c.AUTH_BEARER_TOKEN).toBe("tok");
    expect(c.OTEL_EXPORTER_OTLP_ENDPOINT).toBe("http://localhost:4318");
    expect(c.OTEL_SERVICE_NAME).toBe("edge-x");
  });

  it("rejects a non-http gateway url", () => {
    expect(() => loadConfig({ GATEWAY_URL: "ftp://nope" })).toThrow(ConfigError);
  });

  it("rejects an out-of-range port", () => {
    expect(() => loadConfig({ PORT: "70000" })).toThrow(ConfigError);
    expect(() => loadConfig({ PORT: "not-a-number" })).toThrow(ConfigError);
  });

  it("collects multiple problems in one error", () => {
    try {
      loadConfig({ GATEWAY_URL: "ftp://nope", PORT: "0" });
      throw new Error("expected ConfigError");
    } catch (err) {
      expect(err).toBeInstanceOf(ConfigError);
      expect((err as ConfigError).message).toContain("GATEWAY_URL");
      expect((err as ConfigError).message).toContain("PORT");
    }
  });
});
