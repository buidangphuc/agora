import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WireTrackBeacon } from "./schema";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function stubBrowser() {
  const target = new EventTarget();
  const doc = Object.assign(target, {
    visibilityState: "visible",
    referrer: "",
    cookie: "",
  });
  const win = Object.assign(target, {
    dataLayer: [] as unknown[],
    document: doc,
    location: { pathname: "/", search: "", href: "http://localhost/" },
    sessionStorage: { getItem: () => null, setItem: () => {} },
    localStorage: { getItem: () => null, setItem: () => {} },
  });
  vi.stubGlobal("window", win);
  vi.stubGlobal("document", doc);
  vi.stubGlobal("navigator", { sendBeacon: () => false });
  return { doc };
}

const items = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    itemId: `l${i}`,
    price: 1000 * (i + 1),
    index: i + 1,
    itemListId: "grid",
  }));

const bodies = (fetchMock: ReturnType<typeof vi.fn>): WireTrackBeacon[][] =>
  fetchMock.mock.calls.map((c) => JSON.parse(c[1].body));

describe("trackEcommerce fan-out and batching", () => {
  let fetchMock: ReturnType<typeof vi.fn>;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.resetModules();
    fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 202 });
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("[Fan-out rows share a group id] 5 items -> 5 beacons, distinct event_id, one group id, own position/price/item_list_id", async () => {
    stubBrowser();
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { currency: "VND", items: items(5) });
    await vi.advanceTimersByTimeAsync(2500);

    const sent = bodies(fetchMock).flat();
    expect(sent).toHaveLength(5);
    expect(new Set(sent.map((b) => b.eventId)).size).toBe(5);
    for (const b of sent) expect(b.eventId).toMatch(UUID);
    const groups = new Set(sent.map((b) => b.eventGroupId));
    expect(groups.size).toBe(1);
    expect([...groups][0]).toBeTruthy();
    expect(new Set(sent.map((b) => b.position)).size).toBe(5);
    expect(new Set(sent.map((b) => b.price)).size).toBe(5);
    for (const b of sent) expect(b.itemListId).toBe("grid");
  });

  it("[Fan-out rows share a group id] separate dispatches get different group ids", async () => {
    stubBrowser();
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { items: items(2) });
    trackEcommerce("view_item_list", { items: items(2) });
    await vi.advanceTimersByTimeAsync(2500);
    const sent = bodies(fetchMock).flat();
    expect(sent).toHaveLength(4);
    expect(new Set(sent.map((b) => b.eventGroupId)).size).toBe(2);
  });

  it("[One request per rendered list] 12 items and 2 s idle -> exactly 1 POST with an array of 12", async () => {
    stubBrowser();
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { items: items(12) });
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(2000);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/track$/);
    expect(fetchMock.mock.calls[0][1].method).toBe("POST");
    const body = bodies(fetchMock)[0];
    expect(Array.isArray(body)).toBe(true);
    expect(body).toHaveLength(12);
  });

  it("[Size flush boundary] 19 beacons stay queued; the 20th flushes immediately as one request of 20", async () => {
    stubBrowser();
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { items: items(19) });
    expect(fetchMock).not.toHaveBeenCalled();
    trackEcommerce("view_item", { items: items(1) });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(bodies(fetchMock)[0]).toHaveLength(20);
  });

  it("[Page hidden flushes pending beacons] visibilitychange hidden with 3 queued sends all 3 before any timer", async () => {
    const { doc } = stubBrowser();
    const sendBeacon = vi.fn().mockReturnValue(true);
    vi.stubGlobal("navigator", { sendBeacon });
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { items: items(3) });
    expect(fetchMock).not.toHaveBeenCalled();
    expect(sendBeacon).not.toHaveBeenCalled();

    doc.visibilityState = "hidden";
    doc.dispatchEvent(new Event("visibilitychange"));

    vi.useRealTimers(); // FileReader below needs real timers
    const parse = async (b: unknown) => {
      if (typeof b === "string") return JSON.parse(b);
      return JSON.parse(
        await new Promise<string>((res) => {
          const r = new FileReader();
          r.onload = () => res(String(r.result));
          r.readAsText(b as Blob);
        }),
      );
    };
    const sent: WireTrackBeacon[] = [];
    for (const c of sendBeacon.mock.calls) sent.push(...(await parse(c[1])));
    for (const c of fetchMock.mock.calls) sent.push(...JSON.parse(c[1].body));
    expect(sent).toHaveLength(3);
  });

  it("[Page hidden flushes pending beacons] visibilitychange to visible does not flush", async () => {
    const { doc } = stubBrowser();
    const { trackEcommerce } = await import("./dispatcher");
    trackEcommerce("view_item_list", { items: items(3) });
    doc.visibilityState = "visible";
    doc.dispatchEvent(new Event("visibilitychange"));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("[Failing transport does not surface an error] rejected fetch: no throw into caller, one retry with same body", async () => {
    stubBrowser();
    fetchMock.mockRejectedValue(new Error("network"));
    const { trackEcommerce } = await import("./dispatcher");
    expect(() =>
      trackEcommerce("view_item_list", { items: items(2) }),
    ).not.toThrow();
    await vi.advanceTimersByTimeAsync(2500);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(fetchMock.mock.calls[1][1].body).toBe(
      fetchMock.mock.calls[0][1].body,
    );
  });

  it("[Dispatch during SSR is a no-op] no window: returns without throwing and queues nothing", async () => {
    vi.stubGlobal("window", undefined);
    const { trackEcommerce } = await import("./dispatcher");
    expect(() =>
      trackEcommerce("view_item_list", { items: items(3) }),
    ).not.toThrow();
    await vi.advanceTimersByTimeAsync(5000);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
