import { beforeEach, describe, expect, it, vi } from "vitest";

const { enqueue, enqueueBatch } = vi.hoisted(() => ({
  enqueue: vi.fn(),
  enqueueBatch: vi.fn(),
}));
vi.mock("./queue", () => ({ beaconQueue: { enqueue, enqueueBatch } }));
vi.mock("./dataLayer", () => ({ pushDataLayer: vi.fn() }));

import { trackAiInteraction } from "./ai";

function queued(): Array<Record<string, unknown>> {
  return [
    ...enqueue.mock.calls.map((c) => c[0]),
    ...enqueueBatch.mock.calls.flatMap((c) => c[0]),
  ];
}

beforeEach(() => {
  enqueue.mockClear();
  enqueueBatch.mockClear();
  vi.resetModules();
  vi.doUnmock("./dispatcher");
  vi.doUnmock("@/lib/analytics");
});

// Scenario: The helper sends only allowlisted keys
describe("trackAiInteraction", () => {
  it("sends only surface and action (listing id on the beacon, no other key)", () => {
    trackAiInteraction("magic_listing", "accepted", "lst_1");
    const beacons = queued();
    expect(beacons).toHaveLength(1);
    expect(beacons[0].type).toBe("ai_interaction");
    expect(beacons[0].listingId).toBe("lst_1");
    expect(beacons[0].properties).toEqual({
      surface: "magic_listing",
      action: "accepted",
    });
    expect(Object.keys(beacons[0].properties as object).sort()).toEqual([
      "action",
      "surface",
    ]);
    // No model/placement attribution keys leak onto the beacon.
    for (const k of ["placementId", "modelVersion", "impressionId"]) {
      expect(beacons[0]).not.toHaveProperty(k);
    }
  });

  it("sends only surface and action without a listing id (no listing on the beacon)", () => {
    trackAiInteraction("assistant", "shown");
    const beacons = queued();
    expect(beacons).toHaveLength(1);
    expect(beacons[0].type).toBe("ai_interaction");
    expect(beacons[0].listingId).toBe("");
    expect(beacons[0].properties).toEqual({
      surface: "assistant",
      action: "shown",
    });
  });

  it("sends only surface and action through trackEcommerce('ai_interaction')", async () => {
    const trackEcommerce = vi.fn();
    vi.doMock("./dispatcher", () => ({ trackEcommerce }));
    vi.doMock("@/lib/analytics", () => ({ trackEcommerce }));
    const { trackAiInteraction: track } = await import("./ai");
    track("review_summary", "shown", "lst_2");
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    const [name, params] = trackEcommerce.mock.calls[0];
    expect(name).toBe("ai_interaction");
    expect(params.properties).toEqual({
      surface: "review_summary",
      action: "shown",
    });
    expect(params.items).toEqual([{ itemId: "lst_2" }]);
  });

  // Scenario: A failing tracker never breaks the AI action
  it("swallows tracker failure", async () => {
    const trackEcommerce = vi.fn(() => {
      throw new Error("tracker down");
    });
    vi.doMock("./dispatcher", () => ({ trackEcommerce }));
    vi.doMock("@/lib/analytics", () => ({ trackEcommerce }));
    const { trackAiInteraction: track } = await import("./ai");
    expect(() => track("chat_copilot", "edited", "lst_3")).not.toThrow();
    expect(trackEcommerce).toHaveBeenCalled();
  });

  it("swallows tracker failure when sendBeacon is unavailable", () => {
    enqueue.mockImplementation(() => {
      throw new Error("no sendBeacon");
    });
    try {
      expect(() =>
        trackAiInteraction("assistant", "accepted", "l"),
      ).not.toThrow();
    } finally {
      enqueue.mockReset();
    }
  });
});

describe("trackAiInteraction provenance", () => {
  // Scenario: The helper puts provenance in wire fields only
  it("sends provenance in wire fields only", () => {
    trackAiInteraction("assistant", "shown", undefined, {
      requestId: "r1",
      modelVersion: "mock:v1",
    });
    const beacons = queued();
    expect(beacons).toHaveLength(1);
    expect(beacons[0].type).toBe("ai_interaction");
    expect(beacons[0].impressionId).toBe("r1");
    expect(beacons[0].modelVersion).toBe("mock:v1");
    expect(beacons[0].properties).toEqual({
      surface: "assistant",
      action: "shown",
    });
  });

  // Scenario: Empty provenance sends nothing extra
  it("omits empty provenance", () => {
    trackAiInteraction("review_summary", "shown", "lst_1", {
      requestId: "",
      modelVersion: "",
    });
    const beacons = queued();
    expect(beacons).toHaveLength(1);
    expect(beacons[0]).not.toHaveProperty("impressionId");
    expect(beacons[0]).not.toHaveProperty("modelVersion");
  });

  it("omits an empty model version but keeps the request id", () => {
    trackAiInteraction("chat_copilot", "accepted", "lst_1", {
      requestId: "c1",
      modelVersion: "",
    });
    const beacons = queued();
    expect(beacons[0].impressionId).toBe("c1");
    expect(beacons[0]).not.toHaveProperty("modelVersion");
  });

  // Scenario: A tracker failure still leaves the AI action working
  it("swallows tracker failure with provenance", async () => {
    const trackEcommerce = vi.fn(() => {
      throw new Error("tracker down");
    });
    vi.doMock("./dispatcher", () => ({ trackEcommerce }));
    vi.doMock("@/lib/analytics", () => ({ trackEcommerce }));
    const { trackAiInteraction: track } = await import("./ai");
    expect(() =>
      track("assistant", "shown", undefined, {
        requestId: "r1",
        modelVersion: "m",
      }),
    ).not.toThrow();
  });

  it("supports the dismissed action", () => {
    trackAiInteraction("assistant", "dismissed" as never);
    expect(queued()[0].properties).toEqual({
      surface: "assistant",
      action: "dismissed",
    });
  });
});
