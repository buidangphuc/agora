import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { buildBeacon, track } from "./track.js";

describe("buildBeacon", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it("carries behavioral context only — no authenticated identity (no PII)", () => {
    const beacon = buildBeacon({
      type: "view",
      listingId: "prod-123",
      path: "/listing/prod-123",
    });

    expect(beacon.type).toBe("view");
    expect(beacon.listingId).toBe("prod-123");
    expect(beacon.path).toBe("/listing/prod-123");

    // The payload must never carry authenticated identity — that travels in the
    // gateway envelope principal. Assert no user/email/token-shaped keys leak in.
    const keys = Object.keys(beacon);
    for (const forbidden of [
      "userId",
      "user",
      "email",
      "name",
      "token",
      "principal",
    ]) {
      expect(keys).not.toContain(forbidden);
    }
  });

  it("generates and persists a stable anonymous id + session id", () => {
    const first = buildBeacon({ type: "click", listingId: "a" });
    expect(first.anonymousId).not.toBe("");
    expect(first.sessionId).not.toBe("");

    const second = buildBeacon({ type: "click", listingId: "b" });
    expect(second.anonymousId).toBe(first.anonymousId);
    expect(second.sessionId).toBe(first.sessionId);
  });

  it("includes position and query for search impressions", () => {
    const beacon = buildBeacon({
      type: "impression",
      listingId: "prod-9",
      position: 3,
      query: "iphone",
    });
    expect(beacon.position).toBe(3);
    expect(beacon.query).toBe("iphone");
  });

  it("defaults missing fields without throwing", () => {
    const beacon = buildBeacon({ type: "add_to_cart" });
    expect(beacon.listingId).toBe("");
    expect(beacon.position).toBe(0);
    expect(beacon.query).toBe("");
  });
});

describe("track", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends a beacon via navigator.sendBeacon", () => {
    const sendBeacon = vi.fn().mockReturnValue(true);
    vi.stubGlobal("navigator", { sendBeacon });

    track({ type: "view", listingId: "prod-1", path: "/listing/prod-1" });

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const [url] = sendBeacon.mock.calls[0];
    expect(String(url)).toContain("/api/track");
    vi.unstubAllGlobals();
  });

  it("never throws into the caller when the transport fails", () => {
    const sendBeacon = vi.fn(() => {
      throw new Error("boom");
    });
    // Make the fetch fallback throw too, so both paths fail.
    const fetchMock = vi.fn(() => {
      throw new Error("network down");
    });
    vi.stubGlobal("navigator", { sendBeacon });
    vi.stubGlobal("fetch", fetchMock);

    expect(() => track({ type: "click", listingId: "prod-2" })).not.toThrow();
    vi.unstubAllGlobals();
  });
});
