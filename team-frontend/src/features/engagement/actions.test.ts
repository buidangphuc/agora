import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  addFavorite,
  createCollection,
  removeFavorite,
} from "@/lib/gateway/engagement";

import {
  addFavoriteAction,
  createCollectionResultAction,
  removeFavoriteAction,
} from "./actions";

vi.mock("@/lib/gateway/engagement", () => ({
  addFavorite: vi.fn(),
  removeFavorite: vi.fn(),
  createCollection: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("engagement actions", () => {
  it("addFavoriteAction favorites the listing and revalidates both paths", async () => {
    vi.mocked(addFavorite).mockResolvedValue(undefined);
    await addFavoriteAction("l1");
    expect(addFavorite).toHaveBeenCalledWith("l1");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
    expect(revalidatePath).toHaveBeenCalledWith("/favorites");
  });

  it("removeFavoriteAction unfavorites the listing", async () => {
    vi.mocked(removeFavorite).mockResolvedValue(undefined);
    await removeFavoriteAction("l1");
    expect(removeFavorite).toHaveBeenCalledWith("l1");
    expect(revalidatePath).toHaveBeenCalledWith("/listing/l1");
  });

  it("propagates a gateway error (no local catch)", async () => {
    vi.mocked(addFavorite).mockRejectedValue(new Error("401"));
    await expect(addFavoriteAction("l1")).rejects.toThrow("401");
  });
});

describe("createCollectionResultAction", () => {
  it("creates the collection, revalidates /favorites and returns it as data", async () => {
    const collection = { id: "c1", name: "Mua sau", itemCount: 0 } as never;
    vi.mocked(createCollection).mockResolvedValue(collection);
    await expect(createCollectionResultAction("  Mua sau ")).resolves.toEqual({
      ok: true,
      data: collection,
    });
    expect(createCollection).toHaveBeenCalledWith("Mua sau");
    expect(revalidatePath).toHaveBeenCalledWith("/favorites");
  });

  it("rejects an empty name without calling the gateway", async () => {
    const res = await createCollectionResultAction("   ");
    expect(res.ok).toBe(false);
    expect(createCollection).not.toHaveBeenCalled();
  });

  it("resolves with { ok: false, error } when the gateway throws", async () => {
    vi.mocked(createCollection).mockRejectedValue(new Error("quota"));
    await expect(createCollectionResultAction("x")).resolves.toEqual({
      ok: false,
      error: "quota",
    });
  });
});
