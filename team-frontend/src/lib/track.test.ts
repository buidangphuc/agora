import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const FORBIDDEN_KEY = /(^|_|-)(email|phone|name|address)$|^(email|phone|name|address)(_|$)/i;
const PII_VALUES = ["buyer@example.com", "0901234567", "Nguyen Van A"];

function collectKeys(v: unknown, out: string[] = []): string[] {
  if (Array.isArray(v)) v.forEach((x) => collectKeys(x, out));
  else if (v && typeof v === "object")
    for (const [k, x] of Object.entries(v)) {
      out.push(k);
      collectKeys(x, out);
    }
  return out;
}

describe("track() purchase payload", () => {
  beforeEach(() => {
    vi.resetModules();
    vi.useFakeTimers();
    (window as { dataLayer?: unknown }).dataLayer = [];
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("[Payload keys limited to schema] dataLayer and beacon body carry no buyer email/phone/name", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    const { track } = await import("./track");

    track({
      type: "purchase",
      listingId: "l-1",
      transactionId: "o-1",
      currency: "VND",
      value: 500000,
      price: 500000,
      quantity: 1,
    });
    await vi.advanceTimersByTimeAsync(2500);

    const dl = (window as unknown as { dataLayer: unknown[] }).dataLayer;
    expect(dl.some((e) => (e as { event?: string }).event === "purchase")).toBe(true);
    expect(fetchMock).toHaveBeenCalled();
    const body = String(fetchMock.mock.calls[0][1].body);
    const beacons = JSON.parse(body);

    for (const key of [...collectKeys(dl), ...collectKeys(beacons)]) {
      expect(key).not.toMatch(FORBIDDEN_KEY);
    }
    const serialized = JSON.stringify(dl) + body;
    for (const v of PII_VALUES) expect(serialized).not.toContain(v);
  });
});
