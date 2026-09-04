import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  followSeller,
  isFollowing,
  listFollowedSellers,
  unfollowSeller,
} from "./engagement.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubEngagement(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const engagement = {
    followSeller: vi.fn().mockResolvedValue({}),
    unfollowSeller: vi.fn().mockResolvedValue({}),
    isFollowing: vi.fn(),
    listFollowedSellers: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ engagement } as never);
  return engagement;
}

beforeEach(() => vi.clearAllMocks());

describe("follow-seller gateway wrappers", () => {
  it("followSeller forwards the sellerId", async () => {
    const engagement = stubEngagement({});
    await followSeller("s1");
    expect(engagement.followSeller).toHaveBeenCalledWith({ sellerId: "s1" });
  });

  it("unfollowSeller forwards the sellerId", async () => {
    const engagement = stubEngagement({});
    await unfollowSeller("s1");
    expect(engagement.unfollowSeller).toHaveBeenCalledWith({ sellerId: "s1" });
  });

  it("isFollowing returns the flag and defaults to false on error", async () => {
    stubEngagement({
      isFollowing: vi.fn().mockResolvedValue({ following: true }),
    });
    await expect(isFollowing("s1")).resolves.toBe(true);

    stubEngagement({
      isFollowing: vi.fn().mockRejectedValue(new Error("anon")),
    });
    await expect(isFollowing("s1")).resolves.toBe(false);
  });

  it("listFollowedSellers maps ids and normalizes errors to []", async () => {
    stubEngagement({
      listFollowedSellers: vi
        .fn()
        .mockResolvedValue({ sellerIds: ["a", "b"] }),
    });
    await expect(listFollowedSellers()).resolves.toEqual(["a", "b"]);

    stubEngagement({
      listFollowedSellers: vi.fn().mockRejectedValue(new Error("x")),
    });
    await expect(listFollowedSellers()).resolves.toEqual([]);
  });
});
