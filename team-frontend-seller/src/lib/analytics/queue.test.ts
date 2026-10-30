import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WireTrackBeacon } from "./schema";

const beacon = (eventId: string): WireTrackBeacon => ({
  eventId,
  type: "view",
  listingId: "l1",
  sessionId: "s",
  anonymousId: "a",
  path: "/",
  referrer: "",
  position: 0,
  query: "",
});

describe("beaconQueue fetch fallback retry", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetModules();
    vi.stubGlobal("navigator", { sendBeacon: () => false });
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("re-sends a failed fetch once with the same body", async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error("network"));
    vi.stubGlobal("fetch", fetchMock);
    const { beaconQueue } = await import("./queue");

    beaconQueue.enqueue(beacon("id-1"));
    beaconQueue.flush();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1].body).toBe(
      fetchMock.mock.calls[0][1].body,
    );
    expect(fetchMock.mock.calls[0][1].body).toContain('"eventId":"id-1"');

    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not retry a successful fetch", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 202 });
    vi.stubGlobal("fetch", fetchMock);
    const { beaconQueue } = await import("./queue");

    beaconQueue.enqueue(beacon("id-2"));
    beaconQueue.flush();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

// Scenarios: openspec/changes/add-consent-mode/specs/consent/spec.md
// "Rejected visitor still sends a beacon", "Consent change applies to later beacons only".
describe("beacons are independent of consent", () => {
  const sent: string[] = [];

  beforeEach(() => {
    sent.length = 0;
    vi.resetModules();
    const sendBeacon = vi.fn((_url: string, blob: Blob) => {
      sent.push(String((blob as unknown as { _body?: string })._body));
      return true;
    });
    vi.stubGlobal(
      "Blob",
      class {
        _body: string;
        constructor(parts: string[]) {
          this._body = parts.join("");
        }
      },
    );
    vi.stubGlobal("navigator", { sendBeacon });
    vi.stubGlobal("fetch", vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    document.cookie = "agora_consent=; Path=/; Max-Age=0";
  });

  const bodies = () =>
    sent.map((b) => JSON.parse(b) as Record<string, unknown>[]);

  it("Rejected visitor still sends a beacon: no cookie, body has no consent key", async () => {
    document.cookie = "agora_consent=; Path=/; Max-Age=0";
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    trackEcommerce("view_item", { items: [{ itemId: "l1" }] });
    beaconQueue.flush();
    expect(sent).toHaveLength(1);
    const batch = bodies()[0];
    expect(batch.length).toBeGreaterThan(0);
    for (const b of batch) expect(b).not.toHaveProperty("consent");
    expect(sent[0]).not.toContain("consent");
  });

  it("Rejected visitor still sends a beacon: agora_consent rejects everything", async () => {
    const { writeConsent } = await import("../consent/consent");
    writeConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    expect(document.cookie).toContain("agora_consent=");
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    trackEcommerce("view_item", { items: [{ itemId: "l1" }] });
    beaconQueue.flush();
    expect(sent).toHaveLength(1);
    for (const b of bodies()[0]) expect(b).not.toHaveProperty("consent");
    expect(sent[0]).not.toContain("consent");
  });

  it("Consent change applies to later beacons only: bodies never carry consent", async () => {
    const { writeConsent } = await import("../consent/consent");
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    writeConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    trackEcommerce("view_item", { items: [{ itemId: "before" }] });
    beaconQueue.flush();
    const before = sent[0];

    writeConsent({ analytics: true, marketing: true, personalization: true });
    trackEcommerce("view_item", { items: [{ itemId: "after" }] });
    beaconQueue.flush();

    expect(sent).toHaveLength(2);
    expect(sent[0]).toBe(before); // earlier beacon unaffected
    expect(sent[0]).toContain("before");
    expect(sent[1]).toContain("after");
    for (const batch of bodies())
      for (const b of batch) expect(b).not.toHaveProperty("consent");
    expect(sent[1]).not.toContain("personalization");
  });
});

// Scenarios: openspec/changes/add-ga4-destination/specs/ga4-destination/spec.md
// "Beacon carries the client id with analytics consent", "No consent means no client id",
// "Batched events share the consent decision".
describe("flushed beacon bodies carry gaClientId (ga_client_id scenarios)", () => {
  const sent: string[] = [];
  const clear = (n: string) => {
    document.cookie = `${n}=; Path=/; Max-Age=0`;
  };

  beforeEach(() => {
    sent.length = 0;
    vi.resetModules();
    vi.stubGlobal(
      "Blob",
      class {
        _body: string;
        constructor(parts: string[]) {
          this._body = parts.join("");
        }
      },
    );
    vi.stubGlobal("navigator", {
      sendBeacon: vi.fn((_u: string, blob: Blob) => {
        sent.push(String((blob as unknown as { _body?: string })._body));
        return true;
      }),
    });
    vi.stubGlobal("fetch", vi.fn());
    clear("_ga");
    clear("agora_consent");
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    clear("_ga");
    clear("agora_consent");
  });

  const flat = () =>
    sent.flatMap((b) => JSON.parse(b) as Record<string, unknown>[]);

  it("Beacon carries the client id with analytics consent: JSON key gaClientId", async () => {
    const { writeConsent } = await import("../consent/consent");
    writeConsent({ analytics: true, marketing: false, personalization: false });
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    trackEcommerce("view_item", { items: [{ itemId: "l1" }] });
    beaconQueue.flush();
    expect(flat()[0].gaClientId).toBe("1234567890.1700000000");
  });

  it("No consent means no client id: body never contains the id", async () => {
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    trackEcommerce("view_item", { items: [{ itemId: "l1" }] });
    beaconQueue.flush();
    expect(sent).toHaveLength(1);
    expect(sent[0]).not.toContain("1234567890");
    for (const b of flat()) expect(b.gaClientId ?? "").toBe("");
  });

  it("Batched events share the consent decision: pre-consent beacon empty, later one carries id (one batch)", async () => {
    const { writeConsent } = await import("../consent/consent");
    document.cookie = "_ga=GA1.1.1234567890.1700000000; Path=/";
    const { trackEcommerce } = await import("./dispatcher");
    const { beaconQueue } = await import("./queue");
    writeConsent({
      analytics: false,
      marketing: false,
      personalization: false,
    });
    trackEcommerce("view_item", { items: [{ itemId: "before" }] });
    writeConsent({ analytics: true, marketing: false, personalization: false });
    trackEcommerce("view_item", { items: [{ itemId: "after" }] });
    beaconQueue.flush();
    expect(sent).toHaveLength(1);
    const [first, second] = flat();
    expect(first.listingId).toBe("before");
    expect(first.gaClientId ?? "").toBe("");
    expect(second.listingId).toBe("after");
    expect(second.gaClientId).toBe("1234567890.1700000000");
  });
});
