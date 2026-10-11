import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  listCollectionItems,
  listCollections,
  listFavoriteIds,
} from "@/lib/gateway/engagement";
import { getListing } from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";
import {
  beaconsOf,
  beginBeaconCapture,
  drain,
  endBeaconCapture,
  listIds,
  viewListing,
} from "@/test/listBeacons";

vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/engagement", () => ({
  listCollectionItems: vi.fn(),
  listCollections: vi.fn(),
  listFavoriteIds: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({ getListing: vi.fn() }));
vi.mock("@/features/engagement/actions", () => ({
  createCollectionResultAction: vi.fn(),
}));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import FavoritesPage from "./page";

let fetchMock: ReturnType<typeof vi.fn>;
beforeEach(() => {
  fetchMock = beginBeaconCapture();
  vi.mocked(getPrincipal).mockReturnValue({ id: "u", name: "t", scopes: [] });
  vi.mocked(listCollections).mockResolvedValue([] as never);
  vi.mocked(listFavoriteIds).mockResolvedValue({
    ids: ["l1", "l2"],
    nextCursor: "",
    total: 2,
  });
  vi.mocked(listCollectionItems).mockResolvedValue({
    ids: ["l1"],
    nextCursor: "",
    total: 1,
  });
  vi.mocked(getListing).mockImplementation(
    async (id: string) => viewListing(id) as never,
  );
});
afterEach(endBeaconCapture);

describe("/favorites item list id", () => {
  it("[Fan-out rows share a group id] beacons and view_item_list carry item list id favorites", async () => {
    render(await FavoritesPage({}));
    await drain();
    expect(beaconsOf(fetchMock)).toHaveLength(2);
    expect(listIds(fetchMock)).toEqual({
      beacon: ["favorites"],
      layer: ["favorites"],
    });
  });
});
