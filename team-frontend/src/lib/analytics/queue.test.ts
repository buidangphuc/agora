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
