import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { addFavorite, removeFavorite } from "@/lib/gateway/engagement";

import { addFavoriteAction, removeFavoriteAction } from "./actions";

vi.mock("@/lib/gateway/engagement", () => ({
  addFavorite: vi.fn(),
  removeFavorite: vi.fn(),
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
