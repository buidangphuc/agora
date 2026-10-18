import { describe, expect, it } from "vitest";

import { clientContextFrom } from "./client-context.js";

const hdr = (m: Record<string, string>) => ({
  get: (k: string) => m[k.toLowerCase()] ?? null,
});

describe("clientContextFrom", () => {
  it("uses the single forwarded address and the user agent", () => {
    expect(
      clientContextFrom(
        hdr({ "x-forwarded-for": "203.0.113.7", "user-agent": "Mozilla/5.0" }),
        {},
      ),
    ).toEqual({ ip: "203.0.113.7", userAgent: "Mozilla/5.0" });
  });

  it("ignores client-prepended entries: takes the rightmost with one hop", () => {
    expect(
      clientContextFrom(hdr({ "x-forwarded-for": "1.2.3.4, 203.0.113.7" }), {})
        .ip,
    ).toBe("203.0.113.7");
  });

  it("skips extra trusted hops when TRUSTED_PROXY_HOPS says so", () => {
    const h = hdr({ "x-forwarded-for": "1.2.3.4, 203.0.113.7, 10.0.0.2" });
    expect(clientContextFrom(h, { TRUSTED_PROXY_HOPS: "2" }).ip).toBe(
      "203.0.113.7",
    );
    // Fewer entries than hops: no IP at all (never the client-controlled leftmost).
    expect(
      clientContextFrom(h, { TRUSTED_PROXY_HOPS: "9" }).ip,
    ).toBeUndefined();
  });

  it("drops a value that is not an IP address", () => {
    expect(
      clientContextFrom(hdr({ "x-forwarded-for": "evil, <script>" }), {}).ip,
    ).toBeUndefined();
  });

  it("clips the user agent and tolerates missing headers", () => {
    expect(
      clientContextFrom(hdr({ "user-agent": "a".repeat(500) }), {}).userAgent,
    ).toHaveLength(256);
    expect(clientContextFrom(hdr({}), {})).toEqual({});
  });
});

describe("Session client context: spoofed leftmost X-Forwarded-For is ignored (permission)", () => {
  it("Session client context: spoofed leftmost X-Forwarded-For is ignored - forwards the proxy-appended peer", () => {
    expect(
      clientContextFrom(hdr({ "x-forwarded-for": "6.6.6.6, 198.51.100.4" }), {
        TRUSTED_PROXY_HOPS: "1",
      }).ip,
    ).toBe("198.51.100.4");
  });

  it("Session client context: spoofed leftmost X-Forwarded-For is ignored - fewer entries than hops forwards no IP", () => {
    expect(
      clientContextFrom(hdr({ "x-forwarded-for": "198.51.100.4" }), {
        TRUSTED_PROXY_HOPS: "2",
      }).ip,
    ).toBeUndefined();
  });

  it("Session client context: spoofed leftmost X-Forwarded-For is ignored - selected entry not an IP forwards no IP", () => {
    expect(
      clientContextFrom(hdr({ "x-forwarded-for": "198.51.100.4, not-an-ip" }), {
        TRUSTED_PROXY_HOPS: "1",
      }).ip,
    ).toBeUndefined();
  });

  it("Session client context: missing headers (boundary) - no headers yields an empty context", () => {
    expect(clientContextFrom(hdr({}), { TRUSTED_PROXY_HOPS: "1" })).toEqual({});
  });
});
