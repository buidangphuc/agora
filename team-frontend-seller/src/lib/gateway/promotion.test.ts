import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError, createPromiseClient } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  CampaignStatus,
  DiscountType,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";

import {
  cancelMyCampaign,
  createCampaign,
  deactivateMyShopVoucher,
  listMyCampaigns,
  listMyShopVouchers,
} from "./promotion.js";

vi.mock("@connectrpc/connect", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@connectrpc/connect")>()),
  createPromiseClient: vi.fn(),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => ({ id: "seller-a", name: "A", scopes: [] })),
  SESSION_COOKIE: "session",
}));

const voucherRpc = {
  listMyShopVouchers: vi.fn(),
  deactivateMyShopVoucher: vi.fn(),
};
const flashRpc = {
  listMyCampaigns: vi.fn(),
  createCampaign: vi.fn(),
  cancelMyCampaign: vi.fn(),
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(createPromiseClient).mockImplementation(((svc: {
    typeName: string;
  }) =>
    svc.typeName.endsWith("VoucherService")
      ? voucherRpc
      : svc.typeName.endsWith("FlashSaleService")
        ? flashRpc
        : {}) as never);
});

const NOW = new Date("2026-06-15T12:00:00Z");
const ts = (d: Date) => Timestamp.fromDate(d);
const at = (offsetMs: number) => new Date(NOW.getTime() + offsetMs);
const DAY = 86_400_000;

function voucher(over: Record<string, unknown> = {}) {
  return {
    id: "v1",
    code: "GIAM10",
    scope: VoucherScope.SHOP,
    sellerId: "seller-a",
    discountType: DiscountType.PERCENT,
    discountValue: 10n,
    minSpend: 0n,
    maxDiscount: 0n,
    quota: 100n,
    used: 7n,
    startsAt: ts(at(-DAY)),
    endsAt: ts(at(DAY)),
    active: true,
    ...over,
  };
}

function campaign(over: Record<string, unknown> = {}) {
  return {
    id: "c1",
    listingId: "l1",
    variantId: "",
    salePrice: 50000n,
    stockCap: 10n,
    stockSold: 0n,
    startsAt: ts(at(DAY)),
    endsAt: ts(at(2 * DAY)),
    status: CampaignStatus.SCHEDULED,
    sellerId: "seller-a",
    ...over,
  };
}

describe("listMyShopVouchers", () => {
  it("Vouchers of every state are listed with used and quota: labels active/scheduled/ended/deactivated", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [
        voucher({ id: "a", code: "A", used: 3n, quota: 50n }),
        voucher({
          id: "b",
          code: "B",
          startsAt: ts(at(DAY)),
          endsAt: ts(at(2 * DAY)),
        }),
        voucher({
          id: "c",
          code: "C",
          startsAt: ts(at(-3 * DAY)),
          endsAt: ts(at(-DAY)),
        }),
        voucher({ id: "d", code: "D", active: false }),
      ],
      page: { nextCursor: "nx", total: 4n },
    });
    const res = await listMyShopVouchers({}, NOW);
    expect(res.ok).toBe(true);
    if (!res.ok) return;
    expect(res.items.map((r) => r.stateText)).toEqual([
      "Đang chạy",
      "Sắp diễn ra",
      "Đã kết thúc",
      "Đã tắt",
    ]);
    expect(res.items[0]).toMatchObject({
      id: "a",
      code: "A",
      used: 3,
      quota: 50,
    });
    expect(res.nextCursor).toBe("nx");
  });

  it("voucher and campaign rows carry ISO window dates that the hub can format", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [voucher()],
      page: { nextCursor: "", total: 1n },
    });
    flashRpc.listMyCampaigns.mockResolvedValue({
      campaigns: [campaign()],
      page: { nextCursor: "", total: 1n },
    });
    const vouchers = await listMyShopVouchers({}, NOW);
    const campaigns = await listMyCampaigns({});
    if (!vouchers.ok || !campaigns.ok) throw new Error("expected ok");
    expect(vouchers.items[0].startsAt).toBe(at(-DAY).toISOString());
    expect(vouchers.items[0].endsAt).toBe(at(DAY).toISOString());
    expect(campaigns.items[0].startsAt).toBe(at(DAY).toISOString());
    expect(campaigns.items[0].endsAt).toBe(at(2 * DAY).toISOString());
  });

  it("Other sellers' vouchers are never shown: only the caller-scoped RPC with paging and no seller id", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [],
      page: { nextCursor: "", total: 0n },
    });
    await listMyShopVouchers({ cursor: "cur", pageSize: 20 }, NOW);
    expect(voucherRpc.listMyShopVouchers).toHaveBeenCalledTimes(1);
    expect(voucherRpc.listMyShopVouchers).toHaveBeenCalledWith({
      page: { cursor: "cur", pageSize: 20 },
    });
  });

  it("Vouchers of every state are listed with used and quota: defaults to cursor empty and page size 20", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({ vouchers: [] });
    const res = await listMyShopVouchers({}, NOW);
    expect(voucherRpc.listMyShopVouchers).toHaveBeenCalledWith({
      page: { cursor: "", pageSize: 20 },
    });
    expect(res).toMatchObject({ ok: true, items: [], nextCursor: "" });
  });

  it("voucher state at now == starts_at is active", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [voucher({ startsAt: ts(NOW), endsAt: ts(at(DAY)) })],
    });
    const res = await listMyShopVouchers({}, NOW);
    if (!res.ok) throw new Error("expected ok");
    expect(res.items[0].stateText).toBe("Đang chạy");
  });

  it("voucher state at now == ends_at is active", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [voucher({ startsAt: ts(at(-DAY)), endsAt: ts(NOW) })],
    });
    const res = await listMyShopVouchers({}, NOW);
    if (!res.ok) throw new Error("expected ok");
    expect(res.items[0].stateText).toBe("Đang chạy");
  });

  it("voucher state one second outside the window is scheduled / ended", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [
        voucher({ startsAt: ts(at(1000)), endsAt: ts(at(DAY)) }),
        voucher({ startsAt: ts(at(-DAY)), endsAt: ts(at(-1000)) }),
      ],
    });
    const res = await listMyShopVouchers({}, NOW);
    if (!res.ok) throw new Error("expected ok");
    expect(res.items.map((r) => r.stateText)).toEqual([
      "Sắp diễn ra",
      "Đã kết thúc",
    ]);
  });

  it("voucher state deactivated takes precedence over the time-based states", async () => {
    voucherRpc.listMyShopVouchers.mockResolvedValue({
      vouchers: [
        voucher({
          active: false,
          startsAt: ts(at(DAY)),
          endsAt: ts(at(2 * DAY)),
        }),
        voucher({
          active: false,
          startsAt: ts(at(-2 * DAY)),
          endsAt: ts(at(-DAY)),
        }),
      ],
    });
    const res = await listMyShopVouchers({}, NOW);
    if (!res.ok) throw new Error("expected ok");
    expect(res.items.map((r) => r.stateText)).toEqual(["Đã tắt", "Đã tắt"]);
  });

  it.each([
    [Code.PermissionDenied, "denied"],
    [Code.InvalidArgument, "invalid"],
    [Code.Unavailable, "unavailable"],
  ])(
    "Vouchers of every state are listed with used and quota: ConnectError %s becomes typed result %s",
    async (code, typed) => {
      voucherRpc.listMyShopVouchers.mockRejectedValue(
        new ConnectError("boom", code),
      );
      const res = await listMyShopVouchers({ cursor: "bad" }, NOW);
      expect(res).toMatchObject({ ok: false, code: typed });
    },
  );
});

describe("deactivateMyShopVoucher", () => {
  it("Another seller's voucher id is denied: request carries only the voucher id, PERMISSION_DENIED becomes denied", async () => {
    voucherRpc.deactivateMyShopVoucher.mockRejectedValue(
      new ConnectError("nope", Code.PermissionDenied),
    );
    const res = await deactivateMyShopVoucher("v-foreign");
    expect(voucherRpc.deactivateMyShopVoucher).toHaveBeenCalledWith({
      voucherId: "v-foreign",
    });
    expect(res).toMatchObject({ ok: false, code: "denied" });
  });

  it("Deactivating twice is accepted: an already deactivated voucher returns ok", async () => {
    voucherRpc.deactivateMyShopVoucher.mockResolvedValue({
      voucher: voucher({ active: false }),
    });
    const res = await deactivateMyShopVoucher("v1");
    expect(res.ok).toBe(true);
  });

  it("Another seller's voucher id is denied: NOT_FOUND and UNAVAILABLE map to typed results", async () => {
    voucherRpc.deactivateMyShopVoucher.mockRejectedValueOnce(
      new ConnectError("x", Code.NotFound),
    );
    expect(await deactivateMyShopVoucher("v")).toMatchObject({
      ok: false,
      code: "not_found",
    });
    voucherRpc.deactivateMyShopVoucher.mockRejectedValueOnce(
      new ConnectError("x", Code.Unavailable),
    );
    expect(await deactivateMyShopVoucher("v")).toMatchObject({
      ok: false,
      code: "unavailable",
    });
  });
});

describe("listMyCampaigns", () => {
  it("maps campaign status and sold over cap", async () => {
    flashRpc.listMyCampaigns.mockResolvedValue({
      campaigns: [
        campaign({ id: "s", stockCap: 10n, stockSold: 0n }),
        campaign({
          id: "a",
          stockCap: 20n,
          stockSold: 5n,
          status: CampaignStatus.ACTIVE,
        }),
        campaign({ id: "e", status: CampaignStatus.ENDED }),
        campaign({ id: "x", status: CampaignStatus.CANCELLED }),
      ],
      page: { nextCursor: "", total: 4n },
    });
    const res = await listMyCampaigns({});
    if (!res.ok) throw new Error("expected ok");
    expect(res.items.map((r) => [r.id, r.statusText, r.soldText])).toEqual([
      ["s", "Sắp diễn ra", "0/10"],
      ["a", "Đang diễn ra", "5/20"],
      ["e", "Đã kết thúc", "0/10"],
      ["x", "Đã huỷ", "0/10"],
    ]);
    expect(res.items[0]).toMatchObject({ listingId: "l1", salePrice: 50000 });
  });

  it("Other sellers' campaigns are never shown: request has statuses and paging only, no seller id", async () => {
    flashRpc.listMyCampaigns.mockResolvedValue({ campaigns: [] });
    await listMyCampaigns({
      statuses: [CampaignStatus.CANCELLED],
      cursor: "c2",
      pageSize: 20,
    });
    expect(flashRpc.listMyCampaigns).toHaveBeenCalledTimes(1);
    expect(flashRpc.listMyCampaigns).toHaveBeenCalledWith({
      statuses: [CampaignStatus.CANCELLED],
      page: { cursor: "c2", pageSize: 20 },
    });
  });

  it("Campaigns are listed with status and sold over cap: defaults to no status filter, page size 20", async () => {
    flashRpc.listMyCampaigns.mockResolvedValue({ campaigns: [] });
    await listMyCampaigns({});
    expect(flashRpc.listMyCampaigns).toHaveBeenCalledWith({
      statuses: [],
      page: { cursor: "", pageSize: 20 },
    });
  });

  it("Campaigns are listed with status and sold over cap: UNAVAILABLE becomes a typed result", async () => {
    flashRpc.listMyCampaigns.mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    expect(await listMyCampaigns({})).toMatchObject({
      ok: false,
      code: "unavailable",
    });
  });
});

describe("createCampaign", () => {
  const input = {
    listingId: "l1",
    salePrice: 50000,
    stockCap: 10,
    startsAt: at(DAY),
    endsAt: at(2 * DAY),
    idempotencyKey: "key-12345678",
  };

  it("Valid campaign is created and listed: sends the idempotency key, empty variant and no seller id", async () => {
    flashRpc.createCampaign.mockResolvedValue({ campaign: campaign() });
    const res = await createCampaign(input);
    expect(res.ok).toBe(true);
    expect(flashRpc.createCampaign).toHaveBeenCalledTimes(1);
    expect(flashRpc.createCampaign).toHaveBeenCalledWith({
      listingId: "l1",
      variantId: "",
      salePrice: 50000n,
      stockCap: 10n,
      startsAt: ts(input.startsAt),
      endsAt: ts(input.endsAt),
      idempotencyKey: "key-12345678",
    });
  });

  it.each([
    [Code.PermissionDenied, "denied"],
    [Code.InvalidArgument, "invalid"],
    [Code.FailedPrecondition, "failed_precondition"],
    [Code.AlreadyExists, "already_exists"],
    [Code.Unavailable, "unavailable"],
  ])(
    "Foreign listing id is denied: ConnectError %s becomes %s and keeps the server message",
    async (code, typed) => {
      flashRpc.createCampaign.mockRejectedValue(
        new ConnectError("srv msg", code),
      );
      const res = await createCampaign(input);
      expect(res).toMatchObject({ ok: false, code: typed, message: "srv msg" });
    },
  );
});

describe("cancelMyCampaign", () => {
  it("Another seller's campaign id is denied: request carries only the campaign id", async () => {
    flashRpc.cancelMyCampaign.mockRejectedValue(
      new ConnectError("nope", Code.PermissionDenied),
    );
    const res = await cancelMyCampaign("c-foreign");
    expect(flashRpc.cancelMyCampaign).toHaveBeenCalledWith({
      campaignId: "c-foreign",
    });
    expect(res).toMatchObject({ ok: false, code: "denied" });
  });

  it("Cancelling a campaign that started meanwhile is refused: FAILED_PRECONDITION becomes failed_precondition", async () => {
    flashRpc.cancelMyCampaign.mockRejectedValue(
      new ConnectError("started", Code.FailedPrecondition),
    );
    expect(await cancelMyCampaign("c1")).toMatchObject({
      ok: false,
      code: "failed_precondition",
    });
  });

  it("Cancelling a scheduled campaign: success returns the cancelled row", async () => {
    flashRpc.cancelMyCampaign.mockResolvedValue({
      campaign: campaign({ status: CampaignStatus.CANCELLED }),
    });
    const res = await cancelMyCampaign("c1");
    if (!res.ok) throw new Error("expected ok");
    expect(res.campaign.statusText).toBe("Đã huỷ");
  });
});
