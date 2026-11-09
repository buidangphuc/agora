import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const trackEcommerce = vi.hoisted(() => vi.fn());
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));

import { TrackImpression } from "./TrackImpression";

type Call = [string, { items?: { itemId: string; impressionId?: string }[] }];

const calls = () => trackEcommerce.mock.calls as unknown as Call[];

describe("TrackImpression re-sort (search-attribution: Re-sorting gets a new impression id)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    trackEcommerce.mockClear();
    vi.stubGlobal("IntersectionObserver", undefined);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  async function settle() {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
  }

  it("re-sort: a new impressionId on the same mounted cards fires a new beacon carrying the new id", async () => {
    const ui = (id: string) => (
      <>
        <TrackImpression
          listingId="l1"
          placementId="search_results"
          impressionId={id}
          position={0}
        />
        <TrackImpression
          listingId="l2"
          placementId="search_results"
          impressionId={id}
          position={1}
        />
      </>
    );
    const { rerender } = render(ui("req-A"));
    await settle();
    rerender(ui("req-B"));
    await settle();

    const ids = calls().flatMap(([, p]) =>
      (p.items ?? []).map((i) => `${i.itemId}:${i.impressionId}`),
    );
    expect(ids.sort()).toEqual([
      "l1:req-A",
      "l1:req-B",
      "l2:req-A",
      "l2:req-B",
    ]);
  });

  it("re-sort: batches are keyed by impressionId so two ids never share one view_item_list", async () => {
    const ui = (id: string) => (
      <TrackImpression
        listingId="l1"
        placementId="search_results"
        impressionId={id}
        groupId="g"
        query="ao"
      />
    );
    const { rerender } = render(ui("req-A"));
    rerender(ui("req-B"));
    await settle();

    const perCall = calls().map(([, p]) => [
      ...new Set((p.items ?? []).map((i) => i.impressionId)),
    ]);
    expect(perCall.length).toBe(2);
    for (const set of perCall) expect(set.length).toBe(1);
  });
});
