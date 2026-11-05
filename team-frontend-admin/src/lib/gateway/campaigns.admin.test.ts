import { ScalarType, Timestamp, proto3 } from "@bufbuild/protobuf";
import {
  Code,
  ConnectError,
  createPromiseClient,
  createRouterTransport,
} from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { FlashSaleService } from "@/generated/platform/promotion/v1/promotion_connect.js";
import {
  CampaignStatus,
  type CancelCampaignRequest,
  type CreateCampaignRequest,
  FlashSaleCampaign,
  type ListCampaignsRequest,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import { cancelCampaign, createCampaign, listCampaigns } from "./campaigns.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({ getToken: vi.fn(() => "test-token") }));

const ErrorInfo = proto3.makeMessageType("google.rpc.ErrorInfo", [
  { no: 1, name: "reason", kind: "scalar", T: ScalarType.STRING },
  { no: 2, name: "domain", kind: "scalar", T: ScalarType.STRING },
]);

const withReason = (code: Code, reason: string) =>
  new ConnectError("upstream 10.0.0.7 down", code, undefined, [
    new ErrorInfo({ reason, domain: "promotion.platform" }),
  ]);

function fakeGateway(impl: Record<string, (req: never) => unknown>) {
  const transport = createRouterTransport(({ service }) => {
    service(FlashSaleService, impl as never);
  });
  vi.mocked(makeClients).mockReturnValue({
    flashSale: createPromiseClient(FlashSaleService, transport),
  } as never);
}

const START = new Date("2099-11-01T01:00:00.000Z");
const END = new Date("2099-11-01T05:00:00.000Z");

const stored = new FlashSaleCampaign({
  id: "fs-1",
  listingId: "l-1",
  salePrice: 99000n,
  stockCap: 50n,
  stockSold: 7n,
  startsAt: Timestamp.fromDate(START),
  endsAt: Timestamp.fromDate(END),
  status: CampaignStatus.CANCELLED,
  cancelledAt: Timestamp.fromDate(new Date("2026-11-01T00:00:00.000Z")),
});

const input = {
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  startsAt: START,
  endsAt: END,
};

beforeEach(() => vi.clearAllMocks());

describe("listCampaigns", () => {
  it("sends the status filter and cursor and maps status, cancelledAt and the kill-switch flag", async () => {
    let seen: ListCampaignsRequest | undefined;
    fakeGateway({
      listCampaigns(req: ListCampaignsRequest) {
        seen = req;
        return {
          campaigns: [stored],
          page: { nextCursor: "c2" },
          flashSaleDisabled: true,
        };
      },
    });
    const res = await listCampaigns(["active", "scheduled"], "c1");
    expect(seen?.statuses).toEqual([
      CampaignStatus.ACTIVE,
      CampaignStatus.SCHEDULED,
    ]);
    expect(seen?.page?.cursor).toBe("c1");
    expect(res).toMatchObject({
      status: "ok",
      nextCursor: "c2",
      flashSaleDisabled: true,
    });
    if (res.status === "ok") {
      expect(res.campaigns[0]).toMatchObject({
        id: "fs-1",
        status: "cancelled",
        cancelledAt: "2026-11-01T00:00:00.000Z",
      });
    }
  });

  it("maps gateway refusals without leaking the message", async () => {
    for (const [code, status] of [
      [Code.Unauthenticated, "unauthenticated"],
      [Code.PermissionDenied, "forbidden"],
      [Code.Unavailable, "unavailable"],
    ] as const) {
      fakeGateway({
        listCampaigns() {
          throw new ConnectError("upstream 10.0.0.7 down", code);
        },
      });
      const res = await listCampaigns([], "");
      expect(res).toEqual({ status });
    }
  });
});

describe("createCampaign", () => {
  it("sends the idempotency key with the typed fields", async () => {
    let seen: CreateCampaignRequest | undefined;
    fakeGateway({
      createCampaign(req: CreateCampaignRequest) {
        seen = req;
        return { campaign: stored };
      },
    });
    const res = await createCampaign(input, "retry-key-0001");
    expect(seen?.idempotencyKey).toBe("retry-key-0001");
    expect(seen?.salePrice).toBe(99000n);
    expect(res).toMatchObject({ status: "ok" });
  });

  it.each([
    [Code.AlreadyExists, undefined, "duplicate"],
    [Code.InvalidArgument, undefined, "invalid"],
    [Code.Unavailable, "LISTING_UNAVAILABLE", "listing_unavailable"],
    [Code.Unavailable, "AUDIT_UNAVAILABLE", "audit_unavailable"],
    [Code.Unavailable, "PROMOTION_UNAVAILABLE", "promotion_unavailable"],
    [Code.Unavailable, "SOMETHING_ELSE", "unavailable"],
    [Code.DeadlineExceeded, undefined, "timeout"],
  ] as const)(
    "maps %s with reason %s to %s without the raw message",
    async (code, reason, status) => {
      fakeGateway({
        createCampaign() {
          throw reason
            ? withReason(code, reason)
            : new ConnectError("upstream 10.0.0.7 down", code);
        },
      });
      const res = await createCampaign(input, "retry-key-0001");
      expect(res).toEqual({ status });
      expect(JSON.stringify(res)).not.toContain("10.0.0.7");
    },
  );
});

describe("cancelCampaign", () => {
  it("cancels by id and returns the cancelled campaign", async () => {
    let seen: CancelCampaignRequest | undefined;
    fakeGateway({
      cancelCampaign(req: CancelCampaignRequest) {
        seen = req;
        return { campaign: stored };
      },
    });
    const res = await cancelCampaign("fs-1");
    expect(seen?.campaignId).toBe("fs-1");
    expect(res).toMatchObject({
      status: "ok",
      campaign: { id: "fs-1", status: "cancelled" },
    });
  });

  it("maps an audit outage to audit_unavailable", async () => {
    fakeGateway({
      cancelCampaign() {
        throw withReason(Code.Unavailable, "AUDIT_UNAVAILABLE");
      },
    });
    expect(await cancelCampaign("fs-1")).toEqual({
      status: "audit_unavailable",
    });
  });
});
