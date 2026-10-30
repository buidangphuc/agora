import { Timestamp } from "@bufbuild/protobuf";
import {
  Code,
  ConnectError,
  type ServiceImpl,
  createPromiseClient,
  createRouterTransport,
} from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  FlashSaleService,
  SubscriptionService,
} from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  type CreateCampaignRequest,
  FlashSaleCampaign,
  type ListActiveCampaignsRequest,
  PlanTier,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import {
  createCampaign,
  getActiveFlashSale,
  listActiveCampaigns,
  listPlans,
} from "./campaigns.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({ getToken: vi.fn(() => "test-token") }));

type FlashSaleImpl = ServiceImpl<typeof FlashSaleService>;
type SubscriptionImpl = ServiceImpl<typeof SubscriptionService>;

function fakeGateway(
  flash: Partial<FlashSaleImpl> = {},
  subs: Partial<SubscriptionImpl> = {},
) {
  const transport = createRouterTransport((router) => {
    router.service(FlashSaleService, flash);
    router.service(SubscriptionService, subs);
  });
  vi.mocked(makeClients).mockReturnValue({
    flashSale: createPromiseClient(FlashSaleService, transport),
    subscription: createPromiseClient(SubscriptionService, transport),
  } as never);
}

const START = new Date("2026-11-01T01:00:00.000Z");
const END = new Date("2026-11-01T05:00:00.000Z");

const campaign = new FlashSaleCampaign({
  id: "fs-1",
  listingId: "l-1",
  variantId: "v-1",
  salePrice: 99000n,
  stockCap: 50n,
  stockSold: 7n,
  startsAt: Timestamp.fromDate(START),
  endsAt: Timestamp.fromDate(END),
});

const view = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "v-1",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 7,
  startsAt: START.toISOString(),
  endsAt: END.toISOString(),
};

const input = {
  listingId: "l-1",
  variantId: "v-1",
  salePrice: 99000,
  stockCap: 50,
  startsAt: START,
  endsAt: END,
};

beforeEach(() => vi.clearAllMocks());

describe("listActiveCampaigns", () => {
  it("sends the cursor with page size 20 and maps the page", async () => {
    let seen: ListActiveCampaignsRequest | undefined;
    fakeGateway({
      listActiveCampaigns(req) {
        seen = req;
        return { campaigns: [campaign], page: { nextCursor: "20" } };
      },
    });
    const res = await listActiveCampaigns("c1");
    expect(seen?.page).toMatchObject({ cursor: "c1", pageSize: 20 });
    expect(res).toEqual({ status: "ok", campaigns: [view], nextCursor: "20" });
    expect(makeClients).toHaveBeenCalledWith("test-token");
  });

  it.each([
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.Unavailable, "unavailable"],
    [Code.Internal, "unavailable"],
  ] as const)(
    "maps code %s to %s without the message",
    async (code, status) => {
      fakeGateway({
        listActiveCampaigns() {
          throw new ConnectError("upstream 10.0.0.7 down", code);
        },
      });
      const res = await listActiveCampaigns("");
      expect(res).toEqual({ status });
      expect(JSON.stringify(res)).not.toContain("10.0.0.7");
    },
  );
});

describe("getActiveFlashSale", () => {
  it("returns the active campaign for a listing", async () => {
    fakeGateway({
      getActiveFlashSale: (req) => ({
        active: req.listingId === "l-1",
        campaign,
      }),
    });
    expect(await getActiveFlashSale("l-1")).toEqual({
      status: "ok",
      active: true,
      campaign: view,
    });
  });

  it("reports no active sale", async () => {
    fakeGateway({ getActiveFlashSale: () => ({ active: false }) });
    expect(await getActiveFlashSale("l-2")).toEqual({
      status: "ok",
      active: false,
    });
  });

  it("maps INVALID_ARGUMENT to invalid", async () => {
    fakeGateway({
      getActiveFlashSale() {
        throw new ConnectError("listing_id is required", Code.InvalidArgument);
      },
    });
    expect(await getActiveFlashSale("")).toEqual({ status: "invalid" });
  });
});

describe("listPlans", () => {
  it("maps tiers, prices and features", async () => {
    fakeGateway(
      {},
      {
        listPlans: () => ({
          plans: [
            { id: "p-free", name: PlanTier.FREE, price: 0n, features: ["a"] },
            {
              id: "p-pro",
              name: PlanTier.PRO,
              price: 199000n,
              features: ["b", "c"],
            },
            { id: "p-x", name: PlanTier.UNSPECIFIED, price: 1n, features: [] },
          ],
        }),
      },
    );
    expect(await listPlans()).toEqual({
      status: "ok",
      plans: [
        { id: "p-free", tier: "free", price: 0, features: ["a"] },
        { id: "p-pro", tier: "pro", price: 199000, features: ["b", "c"] },
        { id: "p-x", tier: "unspecified", price: 1, features: [] },
      ],
    });
  });

  it("maps a failure to unavailable", async () => {
    fakeGateway(
      {},
      {
        listPlans() {
          throw new ConnectError("boom", Code.Unavailable);
        },
      },
    );
    expect(await listPlans()).toEqual({ status: "unavailable" });
  });
});

describe("createCampaign", () => {
  it("sends typed fields and returns the stored campaign", async () => {
    let seen: CreateCampaignRequest | undefined;
    fakeGateway({
      createCampaign(req) {
        seen = req;
        return { campaign };
      },
    });
    const res = await createCampaign(input);
    expect(seen?.listingId).toBe("l-1");
    expect(seen?.variantId).toBe("v-1");
    expect(seen?.salePrice).toBe(99000n);
    expect(seen?.stockCap).toBe(50n);
    expect(seen?.startsAt?.toDate().toISOString()).toBe(START.toISOString());
    expect(seen?.endsAt?.toDate().toISOString()).toBe(END.toISOString());
    expect(res).toEqual({ status: "ok", campaign: view });
  });

  it("treats a response without a campaign as unavailable", async () => {
    fakeGateway({ createCampaign: () => ({}) });
    expect(await createCampaign(input)).toEqual({ status: "unavailable" });
  });

  it.each([
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.InvalidArgument, "invalid"],
    [Code.NotFound, "not_found"],
    [Code.DeadlineExceeded, "timeout"],
    [Code.Unavailable, "unavailable"],
    [Code.Internal, "unavailable"],
  ] as const)("maps code %s to %s", async (code, status) => {
    fakeGateway({
      createCampaign() {
        throw new ConnectError("audit trail unavailable", code);
      },
    });
    const res = await createCampaign(input);
    expect(res).toEqual({ status });
    expect(JSON.stringify(res)).not.toContain("audit trail");
  });
});
