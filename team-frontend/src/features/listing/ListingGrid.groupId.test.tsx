import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listEntries,
  viewListing,
} from "@/test/listBeacons";

vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { ListingGrid } from "./ListingGrid";

/** IntersectionObserver whose targets become visible only when the test says so. */
type Obs = { cb: IntersectionObserverCallback; self: IntersectionObserver };
const observers: Obs[] = [];
class StagedIO {
  constructor(private cb: IntersectionObserverCallback) {
    observers.push({ cb, self: this as unknown as IntersectionObserver });
  }
  observe() {}
  disconnect() {}
  unobserve() {}
  takeRecords() {
    return [];
  }
}
function reveal(from: number, to: number) {
  for (const o of observers.slice(from, to)) {
    o.cb([{ isIntersecting: true } as IntersectionObserverEntry], o.self);
  }
}

const items = (n: number, prefix = "l") =>
  Array.from({ length: n }, (_, i) => viewListing(`${prefix}${i + 1}`));

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  observers.length = 0;
  fetchMock = beginBeaconCapture();
  vi.stubGlobal("IntersectionObserver", StagedIO);
});
afterEach(endBeaconCapture);

const groups = () => beaconsOf(fetchMock).map((b) => b.eventGroupId);

describe("ListingGrid group id per list instance", () => {
  it("[Fan-out rows share a group id] impressions arriving in two waves >300 ms apart still share ONE eventGroupId", async () => {
    render(<ListingGrid listings={items(24)} placementId="home_feed" />);
    await act(async () => {
      reveal(0, 12);
      await vi.advanceTimersByTimeAsync(600);
      reveal(12, 24);
    });
    await drain();
    const beacons = beaconsOf(fetchMock);
    expect(beacons).toHaveLength(24);
    const ids = new Set(groups());
    expect(ids.size).toBe(1);
    expect([...ids][0]).toBeTruthy();
    expect(new Set(beacons.map((b) => b.eventId)).size).toBe(24);
    expect(new Set(beacons.map((b) => b.position)).size).toBe(24);
    for (const b of beacons) expect(b.itemListId).toBe("home_feed");
  });

  it("[Fan-out rows share a group id] dataLayer entries stay per batch (two view_item_list) and together hold all 24 items", async () => {
    render(<ListingGrid listings={items(24)} placementId="home_feed" />);
    await act(async () => {
      reveal(0, 12);
      await vi.advanceTimersByTimeAsync(600);
      reveal(12, 24);
    });
    await drain();
    const entries = listEntries();
    expect(entries).toHaveLength(2);
    expect(entries.map((e) => e.ecommerce?.items?.length)).toEqual([12, 12]);
  });

  it("[Fan-out rows share a group id] two different ListingGrid instances get different group ids", async () => {
    render(
      <>
        <ListingGrid listings={items(3, "a")} placementId="home_feed" />
        <ListingGrid listings={items(3, "b")} placementId="recently_viewed" />
      </>,
    );
    await act(async () => {
      reveal(0, observers.length);
    });
    await drain();
    const byList = new Map<unknown, Set<unknown>>();
    for (const b of beaconsOf(fetchMock)) {
      const s = byList.get(b.itemListId) ?? new Set();
      s.add(b.eventGroupId);
      byList.set(b.itemListId, s);
    }
    expect(byList.get("home_feed")?.size).toBe(1);
    expect(byList.get("recently_viewed")?.size).toBe(1);
    expect(new Set(groups()).size).toBe(2);
  });

  it("[Fan-out rows share a group id] remounting the same list gets a new group id", async () => {
    const first = render(
      <ListingGrid listings={items(3)} placementId="home_feed" />,
    );
    await act(async () => {
      reveal(0, observers.length);
    });
    await drain();
    const before = new Set(groups());
    first.unmount();
    fetchMock.mockClear();
    observers.length = 0;
    localStorage.clear();
    sessionStorage.clear();

    render(<ListingGrid listings={items(3)} placementId="home_feed" />);
    await act(async () => {
      reveal(0, observers.length);
    });
    await drain();
    const after = new Set(groups());
    expect(before.size).toBe(1);
    expect(after.size).toBe(1);
    expect([...after][0]).not.toBe([...before][0]);
  });
});
