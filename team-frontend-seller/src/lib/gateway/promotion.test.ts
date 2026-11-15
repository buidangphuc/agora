import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  SponsoredService,
  SubscriptionService,
} from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  AdCampaignStatus,
  PlanTier,
  SubscriptionStatus,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import {
  adCampaignStatusText,
  cancelSubscription,
  getMySubscription,
  listMyAdCampaigns,
  listPlans,
  pauseAdCampaign,
  resumeAdCampaign,
} from "./promotion.js";

const subscriptionRpc = {
  listPlans: vi.fn(),
  subscribe: vi.fn(),
  getEntitlements: vi.fn(),
  getMySubscription: vi.fn(),
  cancelSubscription: vi.fn(),
};

const sponsoredRpc = {
  listMyAdCampaigns: vi.fn(),
  pauseAdCampaign: vi.fn(),
  resumeAdCampaign: vi.fn(),
};

vi.mock("@connectrpc/connect", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@connectrpc/connect")>();
  return {
    ...actual,
    createPromiseClient: vi.fn((service: unknown) =>
      service === SubscriptionService
        ? subscriptionRpc
        : service === SponsoredService
          ? sponsoredRpc
          : {},
    ),
  };
});
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => ({ id: "s1", name: "S", scopes: [] })),
}));

beforeEach(() => vi.clearAllMocks());

const periodEnd = Timestamp.fromDate(new Date("2026-12-13T06:00:00Z"));

function protoSub(over: Record<string, unknown> = {}) {
  return {
    id: "sub1",
    sellerId: "s1",
    planId: "p-pro",
    tier: PlanTier.PRO,
    status: SubscriptionStatus.ACTIVE,
    currentPeriodEnd: periodEnd,
    autoRenew: true,
    ...over,
  };
}

describe("getMySubscription", () => {
  it("Active PRO seller sees period end and renewal state", async () => {
    subscriptionRpc.getMySubscription.mockResolvedValue({
      subscription: protoSub(),
    });
    const sub = await getMySubscription();
    expect(subscriptionRpc.getMySubscription).toHaveBeenCalledTimes(1);
    expect(sub.tier).toBe(PlanTier.PRO);
    expect(sub.currentPeriodEnd).toBe("13/12/2026");
    expect(sub.autoRenew).toBe(true);
  });

  it("Active PRO seller sees period end and renewal state: auto_renew off is reported as off", async () => {
    subscriptionRpc.getMySubscription.mockResolvedValue({
      subscription: protoSub({ autoRenew: false }),
    });
    const sub = await getMySubscription();
    expect(sub.tier).toBe(PlanTier.PRO);
    expect(sub.autoRenew).toBe(false);
  });

  it("FREE seller has no period: a missing subscription is FREE", async () => {
    subscriptionRpc.getMySubscription.mockResolvedValue({});
    const sub = await getMySubscription();
    expect(sub.tier).toBe(PlanTier.FREE);
    expect(sub.currentPeriodEnd).toBeFalsy();
    expect(sub.autoRenew).toBe(false);
  });

  it("Subscription read failure disables the buttons: the error is not swallowed into FREE", async () => {
    subscriptionRpc.getMySubscription.mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    await expect(getMySubscription()).rejects.toBeInstanceOf(ConnectError);
  });
});

describe("cancelSubscription", () => {
  it("Confirmed cancel calls CancelSubscription once with an empty request", async () => {
    subscriptionRpc.cancelSubscription.mockResolvedValue({
      subscription: protoSub({ autoRenew: false }),
    });
    const sub = await cancelSubscription();
    expect(subscriptionRpc.cancelSubscription).toHaveBeenCalledTimes(1);
    expect(subscriptionRpc.cancelSubscription).toHaveBeenCalledWith({});
    expect(sub.autoRenew).toBe(false);
    expect(sub.currentPeriodEnd).toBe("13/12/2026");
  });

  it("Cancel denied by the gateway rejects with the gateway error", async () => {
    subscriptionRpc.cancelSubscription.mockRejectedValue(
      new ConnectError("no", Code.PermissionDenied),
    );
    await expect(cancelSubscription()).rejects.toMatchObject({
      code: Code.PermissionDenied,
    });
  });
});

describe("listPlans", () => {
  it("Paid plan prices show as whole VND per term: termDays is mapped", async () => {
    subscriptionRpc.listPlans.mockResolvedValue({
      plans: [
        {
          id: "p-pro",
          name: PlanTier.PRO,
          price: 199000n,
          features: [],
          termDays: 30,
        },
      ],
    });
    const plans = await listPlans();
    expect(plans[0]).toMatchObject({ price: 199000, termDays: 30 });
  });
});

const created = Timestamp.fromDate(new Date("2026-11-10T06:00:00Z"));

function protoCampaign(over: Record<string, unknown> = {}) {
  return {
    id: "c1",
    sellerId: "s1",
    listingId: "l1",
    budget: 50000n,
    bid: 1000n,
    status: AdCampaignStatus.ACTIVE,
    createdAt: created,
    spent: 0n,
    impressions: 0n,
    clicks: 0n,
    ...over,
  };
}

describe("adCampaignStatusText", () => {
  it("Budget-exhausted campaign cannot resume: PAUSED_BUDGET reads Hết ngân sách", () => {
    expect(adCampaignStatusText(AdCampaignStatus.PAUSED_BUDGET)).toBe(
      "Hết ngân sách",
    );
  });

  it("Campaigns persist after reload: existing statuses keep their labels", () => {
    expect(adCampaignStatusText(AdCampaignStatus.ACTIVE)).toBe("Đang chạy");
    expect(adCampaignStatusText(AdCampaignStatus.PAUSED)).toBe("Tạm dừng");
    expect(adCampaignStatusText(AdCampaignStatus.ENDED)).toBe("Đã kết thúc");
  });
});

describe("listMyAdCampaigns", () => {
  it("Campaigns persist after reload: maps a fresh campaign with zero counters", async () => {
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({
      campaigns: [protoCampaign()],
    });
    const res = await listMyAdCampaigns();
    expect(res.ok).toBe(true);
    if (!res.ok) return;
    expect(res.campaigns).toHaveLength(1);
    expect(res.campaigns[0]).toMatchObject({
      id: "c1",
      listingId: "l1",
      budget: 50000,
      bid: 1000,
      spent: 0,
      impressions: 0,
      clicks: 0,
      status: AdCampaignStatus.ACTIVE,
      statusText: "Đang chạy",
    });
  });

  it("Spend and counters come from the service: spent, impressions and clicks are mapped", async () => {
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({
      campaigns: [protoCampaign({ spent: 1000n, impressions: 3n, clicks: 1n })],
    });
    const res = await listMyAdCampaigns();
    if (!res.ok) throw new Error("expected ok");
    expect(res.campaigns[0]).toMatchObject({
      spent: 1000,
      impressions: 3,
      clicks: 1,
    });
  });

  it("Spend and counters come from the service: int64 at the safe-integer limit is exact", async () => {
    const max = BigInt(Number.MAX_SAFE_INTEGER);
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({
      campaigns: [protoCampaign({ budget: max, spent: max - 1n, clicks: max })],
    });
    const res = await listMyAdCampaigns();
    if (!res.ok) throw new Error("expected ok");
    expect(res.campaigns[0].budget).toBe(Number.MAX_SAFE_INTEGER);
    expect(res.campaigns[0].spent).toBe(Number.MAX_SAFE_INTEGER - 1);
    expect(res.campaigns[0].clicks).toBe(Number.MAX_SAFE_INTEGER);
  });

  it("Campaigns persist after reload: PAUSED_BUDGET row carries the budget text", async () => {
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({
      campaigns: [
        protoCampaign({
          status: AdCampaignStatus.PAUSED_BUDGET,
          budget: 5000n,
          bid: 500n,
          spent: 4600n,
        }),
      ],
    });
    const res = await listMyAdCampaigns();
    if (!res.ok) throw new Error("expected ok");
    expect(res.campaigns[0].statusText).toBe("Hết ngân sách");
    expect(res.campaigns[0].spent).toBe(4600);
  });

  it("Seller with no campaign sees an empty state: empty list is ok, not an error", async () => {
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({ campaigns: [] });
    const res = await listMyAdCampaigns();
    expect(res.ok).toBe(true);
    if (res.ok) expect(res.campaigns).toEqual([]);
  });

  it("List failure degrades only the list: UNAVAILABLE resolves to ok false instead of throwing", async () => {
    sponsoredRpc.listMyAdCampaigns.mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    await expect(listMyAdCampaigns()).resolves.toMatchObject({ ok: false });
  });

  it("List failure degrades only the list: a deadline error resolves to ok false", async () => {
    sponsoredRpc.listMyAdCampaigns.mockRejectedValue(
      new ConnectError("deadline", Code.DeadlineExceeded),
    );
    await expect(listMyAdCampaigns()).resolves.toMatchObject({ ok: false });
  });

  it("List failure degrades only the list: the call is bounded by a 2 s timeout", async () => {
    sponsoredRpc.listMyAdCampaigns.mockResolvedValue({ campaigns: [] });
    await listMyAdCampaigns();
    expect(sponsoredRpc.listMyAdCampaigns).toHaveBeenCalledTimes(1);
    expect(sponsoredRpc.listMyAdCampaigns.mock.calls[0][1]).toEqual({
      timeoutMs: 2000,
    });
  });
});

describe("pauseAdCampaign and resumeAdCampaign", () => {
  it("Pause an active campaign: forwards the id with a 2 s timeout and maps the returned campaign", async () => {
    sponsoredRpc.pauseAdCampaign.mockResolvedValue({
      campaign: protoCampaign({
        status: AdCampaignStatus.PAUSED,
        spent: 2000n,
      }),
    });
    const view = await pauseAdCampaign("c1");
    expect(sponsoredRpc.pauseAdCampaign).toHaveBeenCalledWith(
      { campaignId: "c1" },
      { timeoutMs: 2000 },
    );
    expect(view.status).toBe(AdCampaignStatus.PAUSED);
    expect(view.statusText).toBe("Tạm dừng");
    expect(view.spent).toBe(2000);
  });

  it("Resume a paused campaign: forwards the id with a 2 s timeout and maps the returned campaign", async () => {
    sponsoredRpc.resumeAdCampaign.mockResolvedValue({
      campaign: protoCampaign({ status: AdCampaignStatus.ACTIVE }),
    });
    const view = await resumeAdCampaign("c1");
    expect(sponsoredRpc.resumeAdCampaign).toHaveBeenCalledWith(
      { campaignId: "c1" },
      { timeoutMs: 2000 },
    );
    expect(view.statusText).toBe("Đang chạy");
  });

  it("Another seller's campaign is not found: NOT_FOUND propagates as a ConnectError", async () => {
    sponsoredRpc.pauseAdCampaign.mockRejectedValue(
      new ConnectError("nope", Code.NotFound),
    );
    await expect(pauseAdCampaign("other")).rejects.toMatchObject({
      code: Code.NotFound,
    });
  });

  it("Budget-exhausted campaign cannot resume: FAILED_PRECONDITION propagates as a ConnectError", async () => {
    sponsoredRpc.resumeAdCampaign.mockRejectedValue(
      new ConnectError("budget", Code.FailedPrecondition),
    );
    await expect(resumeAdCampaign("c1")).rejects.toMatchObject({
      code: Code.FailedPrecondition,
    });
  });

  it("Pause an active campaign: a response without a campaign is an error", async () => {
    sponsoredRpc.pauseAdCampaign.mockResolvedValue({});
    await expect(pauseAdCampaign("c1")).rejects.toThrow();
  });

  it("Resume a paused campaign: a response without a campaign is an error", async () => {
    sponsoredRpc.resumeAdCampaign.mockResolvedValue({});
    await expect(resumeAdCampaign("c1")).rejects.toThrow();
  });
});
