import { beforeEach, describe, expect, it, vi } from "vitest";

const { enqueue, enqueueBatch } = vi.hoisted(() => ({
  enqueue: vi.fn(),
  enqueueBatch: vi.fn(),
}));
vi.mock("./queue", () => ({ beaconQueue: { enqueue, enqueueBatch } }));
vi.mock("./dataLayer", () => ({ pushDataLayer: vi.fn() }));

import { trackEcommerce } from "./dispatcher";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

describe("trackEcommerce eventId", () => {
  beforeEach(() => {
    enqueue.mockClear();
    enqueueBatch.mockClear();
  });

  it("stamps a UUID eventId on a single beacon, fresh per call", () => {
    trackEcommerce("view_cart");
    trackEcommerce("view_cart");
    const ids = enqueue.mock.calls.map((c) => c[0].eventId);
    expect(ids).toHaveLength(2);
    for (const id of ids) expect(id).toMatch(UUID);
    expect(ids[0]).not.toBe(ids[1]);
  });

  it("gives every item of a fan-out its own eventId", () => {
    trackEcommerce("view_item_list", {
      items: [{ itemId: "a" }, { itemId: "b" }, { itemId: "c" }],
    });
    const beacons = enqueueBatch.mock.calls[0][0];
    const ids = beacons.map((b: { eventId: string }) => b.eventId);
    expect(new Set(ids).size).toBe(3);
    for (const id of ids) expect(id).toMatch(UUID);
  });
});
