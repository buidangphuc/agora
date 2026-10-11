import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { beaconQueue } from "./analytics/queue";
import { track } from "./track.js";

type Wire = Record<string, unknown>;

/** Route the queue through the fetch fallback so the JSON body is inspectable. */
function captureWire() {
  const fetchMock = vi.fn(() => Promise.resolve(new Response(null)));
  vi.stubGlobal("navigator", { sendBeacon: undefined });
  vi.stubGlobal("fetch", fetchMock);
  return () => {
    beaconQueue.flush();
    const calls = fetchMock.mock.calls as unknown as [
      string,
      { body: string },
    ][];
    return calls.flatMap(([, init]) => JSON.parse(init.body) as Wire[]);
  };
}

describe("track (shim over the analytics dispatcher)", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    beaconQueue.flush();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("carries behavioral context only — no authenticated identity (no PII)", () => {
    const wire = captureWire();
    track({ type: "view", listingId: "prod-123", path: "/listing/prod-123" });

    const [beacon] = wire();
    expect(beacon.type).toBe("view");
    expect(beacon.listingId).toBe("prod-123");
    expect(beacon.path).toBe("/listing/prod-123");

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
    const wire = captureWire();
    track({ type: "click", listingId: "a" });
    track({ type: "click", listingId: "b" });

    const [first, second] = wire();
    expect(first.anonymousId).not.toBe("");
    expect(first.sessionId).not.toBe("");
    expect(second.anonymousId).toBe(first.anonymousId);
    expect(second.sessionId).toBe(first.sessionId);
  });

  it("includes position and query for search impressions", () => {
    const wire = captureWire();
    track({
      type: "impression",
      listingId: "prod-9",
      position: 3,
      query: "iphone",
    });

    const [beacon] = wire();
    expect(beacon.position).toBe(3);
    expect(beacon.query).toBe("iphone");
  });

  it("defaults missing fields without throwing", () => {
    const wire = captureWire();
    track({ type: "add_to_cart" });

    const [beacon] = wire();
    expect(beacon.listingId).toBe("");
    expect(beacon.position).toBe(0);
    expect(beacon.query).toBe("");
  });

  it("sends a beacon via navigator.sendBeacon", () => {
    const sendBeacon = vi.fn().mockReturnValue(true);
    vi.stubGlobal("navigator", { sendBeacon });

    track({ type: "view", listingId: "prod-1", path: "/listing/prod-1" });
    beaconQueue.flush();

    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const [url] = sendBeacon.mock.calls[0];
    expect(String(url)).toContain("/api/track");
  });

  it("never throws into the caller when the transport fails", () => {
    const sendBeacon = vi.fn(() => {
      throw new Error("boom");
    });
    const fetchMock = vi.fn(() => {
      throw new Error("network down");
    });
    vi.stubGlobal("navigator", { sendBeacon });
    vi.stubGlobal("fetch", fetchMock);

    expect(() => {
      track({ type: "click", listingId: "prod-2" });
      beaconQueue.flush();
    }).not.toThrow();
  });

  it("[Payload keys limited to schema] purchase dataLayer entry and beacon carry exactly the schema keys, no buyer PII", () => {
    const wire = captureWire();
    (window as { dataLayer?: unknown[] }).dataLayer = [];
    track({
      type: "purchase",
      listingId: "l-1",
      transactionId: "o-1",
      currency: "VND",
      value: 500000,
      price: 500000,
      quantity: 1,
      position: 2,
      path: "/checkout/success",
    });

    const dl = (window as unknown as { dataLayer: Record<string, unknown>[] })
      .dataLayer;
    const entry = dl.find((e) => e.event === "purchase")!;
    expect(Object.keys(entry).sort()).toEqual(
      ["ecommerce", "event", "page_path"].sort(),
    );
    const ecommerce = entry.ecommerce as Record<string, unknown>;
    expect(Object.keys(ecommerce).sort()).toEqual(
      ["currency", "items", "transaction_id", "value"].sort(),
    );
    const [item] = ecommerce.items as Record<string, unknown>[];
    expect(Object.keys(item).sort()).toEqual(
      ["index", "item_id", "price", "quantity"].sort(),
    );

    const beacons = wire();
    expect(beacons).toHaveLength(1);
    expect(Object.keys(beacons[0]).sort()).toEqual(
      [
        "anonymousId",
        "currency",
        "eventGroupId",
        "eventId",
        "listingId",
        "path",
        "position",
        "price",
        "quantity",
        "query",
        "referrer",
        "sessionId",
        "transactionId",
        "type",
        "value",
      ].sort(),
    );
  });
});
