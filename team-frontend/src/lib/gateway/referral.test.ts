import { Timestamp } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ReferralRewardStatus } from "@/generated/platform/referral/v1/referral_pb.js";

const rpc = vi.hoisted(() => ({
  getMyReferral: vi.fn(),
  listReferralRewards: vi.fn(),
  createReferralCode: vi.fn(),
  redeemReferral: vi.fn(),
}));

vi.mock("@connectrpc/connect", async (importOriginal) => {
  const orig = await importOriginal<typeof import("@connectrpc/connect")>();
  return { ...orig, createPromiseClient: () => rpc };
});
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: () => ({}),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

import { getMyReferral, listReferralRewards } from "./referral.js";

beforeEach(() => vi.clearAllMocks());

describe("getMyReferral", () => {
  it("S17 maps pending_xu and released_xu into the view", async () => {
    rpc.getMyReferral.mockResolvedValue({
      code: "ABC123",
      invitedCount: 3n,
      rewardsTotal: 0n,
      pendingXu: 10000n,
      releasedXu: 20000n,
    });
    await expect(getMyReferral()).resolves.toMatchObject({
      code: "ABC123",
      invitedCount: 3,
      pendingXu: 10000,
      releasedXu: 20000,
    });
  });

  it("S19 falls back to zero totals when the read fails", async () => {
    rpc.getMyReferral.mockRejectedValue(new Error("down"));
    await expect(getMyReferral()).resolves.toMatchObject({
      code: "",
      invitedCount: 0,
      pendingXu: 0,
      releasedXu: 0,
    });
  });
});

describe("listReferralRewards", () => {
  it("S18 maps status and formats release_at as dd/MM/yyyy in Asia/Ho_Chi_Minh", async () => {
    rpc.listReferralRewards.mockResolvedValue({
      rewards: [
        {
          id: "r1",
          amount: 10000n,
          reason: "Bạn bè đăng ký",
          status: ReferralRewardStatus.PENDING,
          releaseAt: new Timestamp({ seconds: 1795118400n }),
        },
        {
          id: "r2",
          amount: 20000n,
          reason: "Bạn bè đăng ký",
          status: ReferralRewardStatus.RELEASED,
        },
        {
          id: "r3",
          amount: 5000n,
          reason: "Bạn bè đăng ký",
          status: ReferralRewardStatus.VOID,
        },
      ],
    });
    const rewards = await listReferralRewards();
    expect(rewards.map((r) => r.status)).toEqual([
      "PENDING",
      "RELEASED",
      "VOID",
    ]);
    expect(rewards[0].releaseAt).toBe("20/11/2026");
    expect(rewards[1].releaseAt).toBe("");
    expect(rewards.map((r) => r.amount)).toEqual([10000, 20000, 5000]);
  });

  it("S20 returns an empty list when the read fails", async () => {
    rpc.listReferralRewards.mockRejectedValue(new Error("down"));
    await expect(listReferralRewards()).resolves.toEqual([]);
  });
});
