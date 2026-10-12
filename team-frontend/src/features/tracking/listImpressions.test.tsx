import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => <button type="button">fav</button>,
}));

import { beaconQueue } from "@/lib/analytics/queue";
import { ListingGrid } from "@/features/listing/ListingGrid";
import { SearchImpressions } from "./SearchImpressions";

type Wire = Record<string, unknown>;
type Entry = { event?: string; ecommerce?: { items?: unknown[] } };

function listings(n: number): ViewListing[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `l${i + 1}`,
    title: `Item ${i + 1}`,
    description: "",
    price: 1000 * (i + 1),
    currency: "VND",
    status: "published",
    sellerId: "s1",
    imageKeys: [],
    imageUrl: `https://img.test/${i + 1}.jpg`,
    categoryId: "c1",
    stock: 5,
    variants: [],
  }));
}

const listEntries = () =>
  ((window as unknown as { dataLayer?: Entry[] }).dataLayer ?? []).filter(
    (e) => e.event === "view_item_list",
  );

/** Every card's IntersectionObserver reports "visible" right after observe(). */
class VisibleIO {
  constructor(private cb: IntersectionObserverCallback) {}
  observe() {
    queueMicrotask(() =>
      this.cb(
        [{ isIntersecting: true } as IntersectionObserverEntry],
        this as unknown as IntersectionObserver,
      ),
    );
  }
  disconnect() {}
  unobserve() {}
  takeRecords() {
    return [];
  }
}

describe.each([
  ["IntersectionObserver reports visible", true],
  ["no IntersectionObserver", false],
])("rendered home_feed grid (%s)", (_label, withIO) => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    vi.useFakeTimers();
    localStorage.clear();
    sessionStorage.clear();
    beaconQueue.flush();
    (window as { dataLayer?: unknown[] }).dataLayer = [];
    document.cookie = `agora_consent=v=1&a=1&m=0&p=0&ts=${Math.floor(Date.now() / 1000)}; Path=/`;
    fetchMock = vi.fn(() => Promise.resolve(new Response(null)));
    vi.stubGlobal("navigator", { sendBeacon: undefined });
    vi.stubGlobal("fetch", fetchMock);
    if (withIO) vi.stubGlobal("IntersectionObserver", VisibleIO);
    else vi.stubGlobal("IntersectionObserver", undefined);
  });
  afterEach(() => {
    document.cookie = "agora_consent=; Max-Age=0; Path=/";
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  async function renderGrid(n: number) {
    render(<ListingGrid listings={listings(n)} placementId="home_feed" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
  }

  it("[dataLayer receives ecommerce items] a grid of 12 pushes ONE view_item_list entry with 12 items", async () => {
    await renderGrid(12);
    const entries = listEntries();
    expect(entries).toHaveLength(1);
    expect(entries[0].ecommerce?.items).toHaveLength(12);
  });

  it("[One request per rendered list] a grid of 12 sends exactly 1 POST /api/track with 12 beacons", async () => {
    await renderGrid(12);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      { body: string },
    ];
    expect(String(url)).toContain("/api/track");
    expect(JSON.parse(init.body) as Wire[]).toHaveLength(12);
  });

  it("[Fan-out rows share a group id] the 12 beacons share one non-empty eventGroupId and have distinct eventIds, positions and item_list_id home_feed", async () => {
    await renderGrid(12);
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      { body: string },
    ];
    const beacons = JSON.parse(init.body) as Wire[];
    const groups = new Set(beacons.map((b) => b.eventGroupId));
    expect(groups.size).toBe(1);
    expect([...groups][0]).toBeTruthy();
    expect(new Set(beacons.map((b) => b.eventId)).size).toBe(12);
    expect(new Set(beacons.map((b) => b.position))).toEqual(
      new Set(Array.from({ length: 12 }, (_, i) => i + 1)),
    );
    for (const b of beacons) expect(b.itemListId).toBe("home_feed");
  });
});

describe("SearchImpressions", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    localStorage.clear();
    sessionStorage.clear();
    beaconQueue.flush();
    (window as { dataLayer?: unknown[] }).dataLayer = [];
    document.cookie = `agora_consent=v=1&a=1&m=0&p=0&ts=${Math.floor(Date.now() / 1000)}; Path=/`;
  });
  afterEach(() => {
    document.cookie = "agora_consent=; Max-Age=0; Path=/";
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("[dataLayer receives ecommerce items][One request per rendered list][Fan-out rows share a group id] 5 ids give one entry with 5 items and one POST of 5 beacons with one group id", async () => {
    const fetchMock = vi.fn(() => Promise.resolve(new Response(null)));
    vi.stubGlobal("navigator", { sendBeacon: undefined });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <SearchImpressions listingIds={["a", "b", "c", "d", "e"]} query="q" />,
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2500);
    });
    const entries = listEntries();
    expect(entries).toHaveLength(1);
    expect(entries[0].ecommerce?.items).toHaveLength(5);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [, init] = fetchMock.mock.calls[0] as unknown as [
      string,
      { body: string },
    ];
    const beacons = JSON.parse(init.body) as Wire[];
    expect(beacons).toHaveLength(5);
    expect(new Set(beacons.map((b) => b.eventGroupId)).size).toBe(1);
  });
});
