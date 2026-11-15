import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({ getPublicBadges: vi.fn() }));

vi.mock("@connectrpc/connect", async (orig) => ({
  ...(await orig<typeof import("@connectrpc/connect")>()),
  createPromiseClient: vi.fn(() => rpc),
}));
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => null),
  SESSION_COOKIE: "session",
}));

import { getPublicBadges } from "./verification.js";

beforeEach(() => vi.clearAllMocks());

describe("getPublicBadges", () => {
  it("[The badge call asks for the shop seller only] sends one GetPublicBadges call with the given user ids", async () => {
    rpc.getPublicBadges.mockResolvedValue({ badges: [] });
    await getPublicBadges(["s1"]);
    expect(rpc.getPublicBadges).toHaveBeenCalledTimes(1);
    expect(rpc.getPublicBadges).toHaveBeenCalledWith({ userIds: ["s1"] });
  });

  it("[Verified seller's shop shows the badge] maps verified=true to true", async () => {
    rpc.getPublicBadges.mockResolvedValue({
      badges: [{ userId: "s1", verified: true }],
    });
    const badges = await getPublicBadges(["s1"]);
    expect(badges.get("s1")).toBe(true);
  });

  it("[Non-verified shops show no badge and no status] maps verified=false and absent sellers to not verified", async () => {
    rpc.getPublicBadges.mockResolvedValue({
      badges: [{ userId: "s1", verified: false }],
    });
    const badges = await getPublicBadges(["s1", "s2"]);
    expect(badges.get("s1")).toBe(false);
    expect(badges.get("s2") ?? false).toBe(false);
  });

  it("[Badge lookup failure does not break the page] propagates the RPC error so the page can fall back", async () => {
    rpc.getPublicBadges.mockRejectedValue(new Error("unavailable"));
    await expect(getPublicBadges(["s1"])).rejects.toThrow("unavailable");
  });
});
