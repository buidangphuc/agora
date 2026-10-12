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

// Scenario: "Rejected visitor still sends a beacon" (consent/spec.md)
describe("trackEcommerce is independent of consent", () => {
  beforeEach(() => {
    enqueue.mockClear();
    enqueueBatch.mockClear();
    document.cookie = "agora_consent=; Path=/; Max-Age=0";
  });

  it("Rejected visitor still sends a beacon: enqueued with no consent key (no cookie)", () => {
    trackEcommerce("view_cart");
    expect(enqueue).toHaveBeenCalledTimes(1);
    expect(enqueue.mock.calls[0][0]).not.toHaveProperty("consent");
  });

  it("Rejected visitor still sends a beacon: enqueued with no consent key (all rejected cookie)", async () => {
    const { writeConsent } = await import("../consent/consent");
    writeConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    trackEcommerce("view_item_list", {
      items: [{ itemId: "a" }, { itemId: "b" }],
    });
    const beacons = enqueueBatch.mock.calls[0][0];
    expect(beacons).toHaveLength(2);
    for (const b of beacons) expect(b).not.toHaveProperty("consent");
  });

  it("Consent change applies to later beacons only: later beacons still carry no body consent", async () => {
    const { writeConsent } = await import("../consent/consent");
    writeConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    trackEcommerce("view_cart");
    writeConsent({ analytics: true, marketing: true, personalization: true });
    trackEcommerce("view_cart");
    expect(enqueue).toHaveBeenCalledTimes(2);
    for (const c of enqueue.mock.calls)
      expect(c[0]).not.toHaveProperty("consent");
  });
});

// Scenarios: openspec/changes/add-ga4-destination/specs/ga4-destination/spec.md
// requirement "ga_client_id in first-party beacons".
describe("trackEcommerce gaClientId (ga_client_id scenarios)", () => {
  const clear = (n: string) => {
    document.cookie = `${n}=; Path=/; Max-Age=0`;
  };
  const grant = async (analytics: boolean) => {
    const { writeConsent } = await import("../consent/consent");
    writeConsent({ analytics, marketing: false, personalization: false });
  };
  const last = () => enqueue.mock.calls.at(-1)?.[0];

  beforeEach(() => {
    enqueue.mockClear();
    enqueueBatch.mockClear();
    clear("_ga");
    clear("agora_consent");
  });

  it("Beacon carries the client id with analytics consent", async () => {
    await grant(true);
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    trackEcommerce("view_cart");
    expect(last().gaClientId).toBe("1234567890.1700000000");
  });

  it("Beacon carries the client id with analytics consent: fan-out beacons too", async () => {
    await grant(true);
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    trackEcommerce("view_item_list", {
      items: [{ itemId: "a" }, { itemId: "b" }],
    });
    for (const b of enqueueBatch.mock.calls[0][0])
      expect(b.gaClientId).toBe("1234567890.1700000000");
  });

  it("No consent means no client id: analytics denied", async () => {
    await grant(false);
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    trackEcommerce("view_cart");
    expect(last().gaClientId ?? "").toBe("");
  });

  it("No consent means no client id: agora_consent absent", () => {
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    trackEcommerce("view_cart");
    expect(last().gaClientId ?? "").toBe("");
  });

  it.each([
    ["absent", null],
    ["wrong prefix", "XX1.1.1.2"],
    ["too few parts", "GA1.1.123"],
    ["non-numeric version", "GA1.x.1.2"],
    ["empty", ""],
    ["non-numeric both parts", "GA1.1.abc.def"],
    ["non-numeric second part", "GA1.1.123.abc"],
    ["non-numeric first part", "GA1.1.abc.123"],
    ["three id segments", "GA1.1.1.2.3"],
  ])(
    "Malformed or missing _ga cookie: %s -> empty, no throw",
    async (_n, v) => {
      await grant(true);
      if (v !== null) document.cookie = `_ga=${v}; Path=/`;
      expect(() => trackEcommerce("view_cart")).not.toThrow();
      expect(last().gaClientId ?? "").toBe("");
    },
  );

  it("Batched events share the consent decision: each beacon uses its own enqueue-time consent", async () => {
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    await grant(false);
    trackEcommerce("view_cart");
    await grant(true);
    trackEcommerce("view_cart");
    expect(enqueue.mock.calls[0][0].gaClientId ?? "").toBe("");
    expect(enqueue.mock.calls[1][0].gaClientId).toBe("1234567890.1700000000");
  });

  it("gaClientId is bounded to 64 characters: exactly 64 kept, 65 dropped", async () => {
    await grant(true);
    const mk = (n: number) => {
      const a = "1".repeat(n - 2); // "<a>.<b>" with b = "2"
      return `${a}.2`;
    };
    document.cookie = `_ga=GA1.1.${mk(64)}; Path=/`;
    trackEcommerce("view_cart");
    expect(last().gaClientId).toBe(mk(64));
    expect(last().gaClientId).toHaveLength(64);

    document.cookie = `_ga=GA1.1.${mk(65)}; Path=/`;
    trackEcommerce("view_cart");
    expect(last().gaClientId ?? "").toBe("");
  });
});
