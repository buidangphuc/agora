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

import { ListingGrid } from "@/features/listing/ListingGrid";

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

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  observers.length = 0;
  fetchMock = beginBeaconCapture();
  vi.stubGlobal("IntersectionObserver", StagedIO);
});
afterEach(endBeaconCapture);

const N = 6;
const seq = Array.from({ length: N }, (_, i) => i + 1);

describe("view_item_list item order", () => {
  it("[Tracking hooks are unchanged] cards revealed in reverse order within one window yield dataLayer items ordered by index 1..N", async () => {
    render(
      <ListingGrid
        listings={seq.map((i) => viewListing(`l${i}`))}
        placementId="home_feed"
      />,
    );
    await act(async () => {
      for (const o of [...observers].reverse()) {
        o.cb([{ isIntersecting: true } as IntersectionObserverEntry], o.self);
      }
    });
    await drain();
    const entries = listEntries();
    expect(entries).toHaveLength(1);
    const items = (entries[0].ecommerce?.items ?? []) as { index?: number }[];
    expect(items.map((i) => i.index)).toEqual(seq);
  });

  it("[Tracking hooks are unchanged] cards revealed in reverse order within one window yield beacons ordered by position 1..N", async () => {
    render(
      <ListingGrid
        listings={seq.map((i) => viewListing(`l${i}`))}
        placementId="home_feed"
      />,
    );
    await act(async () => {
      for (const o of [...observers].reverse()) {
        o.cb([{ isIntersecting: true } as IntersectionObserverEntry], o.self);
      }
    });
    await drain();
    expect(beaconsOf(fetchMock).map((b) => b.position)).toEqual(seq);
  });
});
