import { act } from "@testing-library/react";
import { vi } from "vitest";

import { beaconQueue } from "@/lib/analytics/queue";
import type { ViewListing } from "@/lib/gateway/listings";

type Entry = {
  event?: string;
  ecommerce?: { items?: { item_list_id?: string }[] };
};
export type Beacon = Record<string, unknown>;

export function viewListing(id: string): ViewListing {
  return {
    id,
    title: `SP ${id}`,
    description: "",
    price: 1000,
    currency: "VND",
    status: "published",
    sellerId: "s1",
    imageKeys: [],
    imageUrl: "https://img.test/x.jpg",
    categoryId: "c1",
    stock: 5,
    variants: [],
  };
}

/** Fake timers + captured fetch; no IntersectionObserver so cards fire on mount. */
export function beginBeaconCapture() {
  vi.useFakeTimers();
  localStorage.clear();
  sessionStorage.clear();
  beaconQueue.flush();
  (window as { dataLayer?: unknown[] }).dataLayer = [];
  const fetchMock = vi.fn(() => Promise.resolve(new Response(null)));
  vi.stubGlobal("navigator", { sendBeacon: undefined });
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("IntersectionObserver", undefined);
  return fetchMock;
}

export function endBeaconCapture() {
  vi.useRealTimers();
  vi.unstubAllGlobals();
}

export async function drain() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2500);
  });
}

export function listEntries(): Entry[] {
  return (
    (window as unknown as { dataLayer?: Entry[] }).dataLayer ?? []
  ).filter((e) => e.event === "view_item_list");
}

export function beaconsOf(fetchMock: ReturnType<typeof vi.fn>): Beacon[] {
  return fetchMock.mock.calls.flatMap((c) => {
    const init = (c as unknown as [string, { body: string }])[1];
    return JSON.parse(init.body) as Beacon[];
  });
}

/** Asserts-by-data: distinct list ids seen on beacons and on dataLayer items. */
export function listIds(fetchMock: ReturnType<typeof vi.fn>) {
  return {
    beacon: [...new Set(beaconsOf(fetchMock).map((b) => b.itemListId))],
    layer: [
      ...new Set(
        listEntries().flatMap((e) =>
          (e.ecommerce?.items ?? []).map((i) => i.item_list_id),
        ),
      ),
    ],
  };
}
