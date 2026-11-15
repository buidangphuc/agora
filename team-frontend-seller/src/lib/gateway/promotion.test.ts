import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SubscriptionService } from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  PlanTier,
  SubscriptionStatus,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import {
  cancelSubscription,
  getMySubscription,
  listPlans,
} from "./promotion.js";

const subscriptionRpc = {
  listPlans: vi.fn(),
  subscribe: vi.fn(),
  getEntitlements: vi.fn(),
  getMySubscription: vi.fn(),
  cancelSubscription: vi.fn(),
};

vi.mock("@connectrpc/connect", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@connectrpc/connect")>();
  return {
    ...actual,
    createPromiseClient: vi.fn((service: unknown) =>
      service === SubscriptionService ? subscriptionRpc : {},
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
