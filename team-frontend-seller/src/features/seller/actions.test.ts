import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import {
  DiscountType,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";
import {
  createBundle,
  getStorefront,
  listMyListings,
  upsertStorefront,
} from "@/lib/gateway/listings";
import { requestWalletPayout } from "@/lib/gateway/payment";
import {
  cancelMyCampaign,
  cancelSubscription,
  createAdCampaign,
  createCampaign,
  createVoucher,
  deactivateMyShopVoucher,
  pauseAdCampaign,
  resumeAdCampaign,
  subscribe,
} from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import {
  cancelCampaignAction,
  cancelSubscriptionAction,
  createAdCampaignAction,
  createBundleAction,
  createCampaignAction,
  createShopVoucherAction,
  deactivateShopVoucherAction,
  pauseAdCampaignAction,
  requestWalletPayoutAction,
  resumeAdCampaignAction,
  searchMyListingsAction,
  subscribeAction,
  upsertStorefrontAction,
} from "./actions";

vi.mock("@/lib/gateway/listings", () => ({
  createBundle: vi.fn(),
  getStorefront: vi.fn(),
  upsertStorefront: vi.fn(),
  listMyListings: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({ requestWalletPayout: vi.fn() }));
vi.mock("@/lib/gateway/promotion", () => ({
  createAdCampaign: vi.fn(),
  pauseAdCampaign: vi.fn(),
  resumeAdCampaign: vi.fn(),
  subscribe: vi.fn(),
  cancelSubscription: vi.fn(),
  createVoucher: vi.fn(),
  createCampaign: vi.fn(),
  cancelMyCampaign: vi.fn(),
  deactivateMyShopVoucher: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "Seller-ID-123",
    name: "S",
    scopes: ["listing.write"],
  });
});

describe("requestWalletPayoutAction", () => {
  it("returns ok and revalidates the wallet", async () => {
    vi.mocked(requestWalletPayout).mockResolvedValue(null);
    expect(await requestWalletPayoutAction("s1", 5000)).toEqual({ ok: true });
    expect(requestWalletPayout).toHaveBeenCalledWith("s1", 5000);
    expect(revalidatePath).toHaveBeenCalledWith("/seller/wallet");
  });

  it("rejects a non-positive amount without calling the gateway", async () => {
    const res = await requestWalletPayoutAction("s1", 0);
    expect(res).toEqual({ ok: false, error: "Số tiền rút phải lớn hơn 0." });
    expect(requestWalletPayout).not.toHaveBeenCalled();
  });

  it("returns the error field on failure", async () => {
    vi.mocked(requestWalletPayout).mockRejectedValue(new Error("insufficient"));
    expect(await requestWalletPayoutAction("s1", 5)).toEqual({
      ok: false,
      error: "insufficient",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("seller-kyc-ui: requestWalletPayoutAction error mapping", () => {
  it("A refused payout shows the verification message: FAILED_PRECONDITION maps by code, raw text hidden", async () => {
    vi.mocked(requestWalletPayout).mockRejectedValue(
      new ConnectError("seller verification required", Code.FailedPrecondition),
    );
    const res = await requestWalletPayoutAction("s1", 5000);
    expect(res).toEqual({
      ok: false,
      error: "Cần xác minh danh tính để rút tiền",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("A refused payout shows the verification message: mapped by code, not by server text", async () => {
    vi.mocked(requestWalletPayout).mockRejectedValue(
      new ConnectError("something else entirely", Code.FailedPrecondition),
    );
    expect(await requestWalletPayoutAction("s1", 5000)).toEqual({
      ok: false,
      error: "Cần xác minh danh tính để rút tiền",
    });
  });

  it("Verification outage during payout shows a retry message: UNAVAILABLE", async () => {
    vi.mocked(requestWalletPayout).mockRejectedValue(
      new ConnectError("kyc backend down", Code.Unavailable),
    );
    expect(await requestWalletPayoutAction("s1", 5000)).toEqual({
      ok: false,
      error: "Chưa kiểm tra được trạng thái xác minh, vui lòng thử lại sau",
    });
  });

  it("A verified seller sees no notice and can pay out: success unchanged", async () => {
    vi.mocked(requestWalletPayout).mockResolvedValue(null);
    expect(await requestWalletPayoutAction("s1", 5000)).toEqual({ ok: true });
  });
});

describe("subscribeAction", () => {
  it("revalidates plans on success", async () => {
    vi.mocked(subscribe).mockResolvedValue({ ok: true });
    expect(await subscribeAction("p1")).toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/plans");
  });

  it("maps the failure message to error", async () => {
    vi.mocked(subscribe).mockResolvedValue({ ok: false, message: "nope" });
    expect(await subscribeAction("p1")).toEqual({ ok: false, error: "nope" });
  });
});

describe("createBundleAction", () => {
  it("validates with the error field", async () => {
    expect(await createBundleAction("  ", ["a", "b"], 10)).toEqual({
      ok: false,
      error: "Nhập tên combo.",
    });
    expect(await createBundleAction("x", ["a"], 10)).toEqual({
      ok: false,
      error: "Chọn ít nhất 2 sản phẩm cho combo.",
    });
    expect(await createBundleAction("x", ["a", "b"], 0)).toEqual({
      ok: false,
      error: "Giá combo phải lớn hơn 0.",
    });
    expect(createBundle).not.toHaveBeenCalled();
  });

  it("returns the bundle in data and revalidates", async () => {
    const bundle = { id: "b1", title: "x" };
    vi.mocked(createBundle).mockResolvedValue(bundle as never);
    expect(await createBundleAction("x", ["a", "b"], 10)).toEqual({
      ok: true,
      data: { bundle },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/bundles");
  });
});

describe("createAdCampaignAction", () => {
  it("validates and returns the campaign in data", async () => {
    expect(await createAdCampaignAction("", 1, 1)).toMatchObject({ ok: false });
    expect(await createAdCampaignAction("l", 0, 1)).toMatchObject({
      ok: false,
    });
    const campaign = { id: "c1" };
    vi.mocked(createAdCampaign).mockResolvedValue(campaign as never);
    expect(await createAdCampaignAction("l", 5000, 500)).toEqual({
      ok: true,
      data: { campaign },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/ads");
  });

  it("rejects bid below 500 without calling the service (Bid below minimum is rejected)", async () => {
    const res = await createAdCampaignAction("l", 5000, 499);
    expect(res).toEqual({ ok: false, error: "Giá thầu tối thiểu là ₫500." });
    expect(createAdCampaign).not.toHaveBeenCalled();
  });

  it("accepts a bid of exactly 500 (Bid of exactly 500 is accepted)", async () => {
    const campaign = { id: "c1", bid: 500 };
    vi.mocked(createAdCampaign).mockResolvedValue(campaign as never);
    expect(await createAdCampaignAction("l", 5000, 500)).toEqual({
      ok: true,
      data: { campaign },
    });
    expect(createAdCampaign).toHaveBeenCalledWith("l", 5000, 500);
  });
});

describe("pauseAdCampaignAction", () => {
  it("pauses and returns the refreshed campaign (Pause an active campaign)", async () => {
    const campaign = { id: "c1", statusText: "Tạm dừng" };
    vi.mocked(pauseAdCampaign).mockResolvedValue(campaign as never);
    expect(await pauseAdCampaignAction("c1")).toEqual({
      ok: true,
      data: { campaign },
    });
    expect(pauseAdCampaign).toHaveBeenCalledWith("c1");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/ads");
  });

  it("pause maps NOT_FOUND to the not-found message (Another seller's campaign is not found)", async () => {
    vi.mocked(pauseAdCampaign).mockRejectedValue(
      new ConnectError("campaign not found", Code.NotFound),
    );
    expect(await pauseAdCampaignAction("other")).toEqual({
      ok: false,
      error: "Không tìm thấy chiến dịch.",
    });
  });
});

describe("resumeAdCampaignAction", () => {
  it("resumes and returns the refreshed campaign (Resume a paused campaign)", async () => {
    const campaign = { id: "c1", statusText: "Đang chạy" };
    vi.mocked(resumeAdCampaign).mockResolvedValue(campaign as never);
    expect(await resumeAdCampaignAction("c1")).toEqual({
      ok: true,
      data: { campaign },
    });
    expect(resumeAdCampaign).toHaveBeenCalledWith("c1");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/ads");
  });

  it("resume maps NOT_FOUND to the not-found message (Another seller's campaign is not found)", async () => {
    vi.mocked(resumeAdCampaign).mockRejectedValue(
      new ConnectError("campaign not found", Code.NotFound),
    );
    expect(await resumeAdCampaignAction("other")).toEqual({
      ok: false,
      error: "Không tìm thấy chiến dịch.",
    });
  });

  it("resume maps FAILED_PRECONDITION to the budget message (Budget-exhausted campaign cannot resume)", async () => {
    vi.mocked(resumeAdCampaign).mockRejectedValue(
      new ConnectError("remaining below bid", Code.FailedPrecondition),
    );
    expect(await resumeAdCampaignAction("c1")).toEqual({
      ok: false,
      error: "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy.",
    });
  });
});

const current = {
  sellerId: "Seller-ID-123",
  slug: "tiem-hoa",
  bannerUrl: "b.png",
  tagline: "Hoa tươi",
  featuredListingIds: ["a", "b", "c"],
  theme: "festive",
  displayName: "Tiệm Hoa",
};

const input = {
  displayName: "Tiệm Hoa",
  bannerUrl: "b.png",
  tagline: "Hoa tươi",
  featuredListingIds: ["a", "b", "c"],
  theme: "festive",
};

function savedEcho() {
  vi.mocked(upsertStorefront).mockImplementation(
    async (sf) => ({ ...current, ...sf }) as never,
  );
}

describe("upsertStorefrontAction", () => {
  beforeEach(() => {
    vi.mocked(getStorefront).mockResolvedValue(current);
    savedEcho();
  });

  it("Seller shop UI: saving sends the full storefront once - upsertStorefrontAction keeps unchanged fields", async () => {
    const res = await upsertStorefrontAction({ ...input, tagline: "Mới" });
    expect(res.ok).toBe(true);
    expect(getStorefront).toHaveBeenCalledWith("Seller-ID-123", {
      throwOnError: true,
    });
    expect(upsertStorefront).toHaveBeenCalledTimes(1);
    expect(upsertStorefront).toHaveBeenCalledWith({
      sellerId: "Seller-ID-123",
      slug: "tiem-hoa",
      bannerUrl: "b.png",
      tagline: "Mới",
      featuredListingIds: ["a", "b", "c"],
      theme: "festive",
      displayName: "Tiệm Hoa",
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller");
    expect(revalidatePath).toHaveBeenCalledWith("/shop/Seller-ID-123");
  });

  it("Seller shop UI: saving sends the full storefront once - derives slug when no storefront exists", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    await upsertStorefrontAction(input);
    expect(upsertStorefront).toHaveBeenCalledWith(
      expect.objectContaining({
        sellerId: "Seller-ID-123",
        slug: "shop-sellerid123",
        tagline: "Hoa tươi",
        theme: "festive",
      }),
    );
  });

  it("Seller shop UI: a tampered seller id is ignored - seller id comes from the session", async () => {
    await upsertStorefrontAction({
      ...input,
      sellerId: "Other-Seller",
    } as never);
    expect(upsertStorefront).toHaveBeenCalledTimes(1);
    expect(vi.mocked(upsertStorefront).mock.calls[0]?.[0].sellerId).toBe(
      "Seller-ID-123",
    );
    expect(JSON.stringify(vi.mocked(getStorefront).mock.calls)).not.toContain(
      "Other-Seller",
    );
    expect(
      JSON.stringify(vi.mocked(upsertStorefront).mock.calls),
    ).not.toContain("Other-Seller");
    expect(JSON.stringify(vi.mocked(revalidatePath).mock.calls)).not.toContain(
      "Other-Seller",
    );
  });

  it("Seller shop UI: a principal without listing.write cannot save - storefront save refused without scope", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    const anon = await upsertStorefrontAction(input);
    expect(anon).toEqual({
      ok: false,
      error: "Bạn cần đăng nhập để cập nhật gian hàng.",
    });
    vi.mocked(getPrincipal).mockReturnValue({
      id: "Seller-ID-123",
      name: "S",
      scopes: ["orders.read"],
    });
    const noScope = await upsertStorefrontAction(input);
    expect(noScope.ok).toBe(false);
    if (!noScope.ok) {
      expect(noScope.error).not.toBe("");
      expect(noScope.error).not.toBe(
        "Bạn cần đăng nhập để cập nhật gian hàng.",
      );
    }
    expect(getStorefront).not.toHaveBeenCalled();
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("Seller shop UI: a failed read blocks the save - read failure blocks the save", async () => {
    vi.mocked(getStorefront).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    expect(await upsertStorefrontAction(input)).toEqual({
      ok: false,
      error: "Không tải được hồ sơ gian hàng hiện tại. Vui lòng thử lại.",
    });
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("Seller shop UI: an empty tagline clears the tagline only - empty tagline keeps other fields", async () => {
    await upsertStorefrontAction({ ...input, tagline: "" });
    expect(upsertStorefront).toHaveBeenCalledWith({
      sellerId: "Seller-ID-123",
      slug: "tiem-hoa",
      bannerUrl: "b.png",
      tagline: "",
      featuredListingIds: ["a", "b", "c"],
      theme: "festive",
      displayName: "Tiệm Hoa",
    });
  });

  it("Seller shop UI: removing the banner clears it - remove banner sends empty url", async () => {
    await upsertStorefrontAction({ ...input, bannerUrl: "" });
    expect(upsertStorefront).toHaveBeenCalledWith({
      sellerId: "Seller-ID-123",
      slug: "tiem-hoa",
      bannerUrl: "",
      tagline: "Hoa tươi",
      featuredListingIds: ["a", "b", "c"],
      theme: "festive",
      displayName: "Tiệm Hoa",
    });
  });

  it("Seller shop UI: reordering changes the saved order - featured order is kept", async () => {
    await upsertStorefrontAction({
      ...input,
      featuredListingIds: ["a", "c", "b"],
    });
    expect(
      vi.mocked(upsertStorefront).mock.calls[0]?.[0].featuredListingIds,
    ).toEqual(["a", "c", "b"]);
  });

  it("Seller shop UI: a 120-character tagline is accepted and 121 is refused - action re-validates", async () => {
    expect(
      (await upsertStorefrontAction({ ...input, tagline: "a".repeat(120) })).ok,
    ).toBe(true);
    vi.mocked(upsertStorefront).mockClear();
    expect(
      await upsertStorefrontAction({ ...input, tagline: "a".repeat(121) }),
    ).toEqual({ ok: false, error: "Tagline tối đa 120 ký tự." });
    expect(await upsertStorefrontAction({ ...input, tagline: "a\nb" })).toEqual(
      {
        ok: false,
        error: "Tagline không được chứa ký tự điều khiển.",
      },
    );
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("Seller shop UI: The seventh featured listing is refused - action refuses more than six ids", async () => {
    const six = ["1", "2", "3", "4", "5", "6"];
    expect(
      (await upsertStorefrontAction({ ...input, featuredListingIds: six })).ok,
    ).toBe(true);
    vi.mocked(upsertStorefront).mockClear();
    const res = await upsertStorefrontAction({
      ...input,
      featuredListingIds: [...six, "7"],
    });
    expect(res).toEqual({ ok: false, error: "Tối đa 6 sản phẩm nổi bật." });
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("Seller shop UI: a theme outside the set is refused", async () => {
    const res = await upsertStorefrontAction({ ...input, theme: "warm" });
    expect(res.ok).toBe(false);
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("Seller shop UI: a featured listing of another seller is rejected by the server - error mapping", async () => {
    vi.mocked(upsertStorefront).mockRejectedValue(
      new ConnectError(
        "featured listing does not belong to seller",
        Code.InvalidArgument,
      ),
    );
    const res = await upsertStorefrontAction({
      ...input,
      featuredListingIds: ["foreign"],
    });
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.error).toContain("featured listing does not belong to seller");
    }
    expect(revalidatePath).not.toHaveBeenCalledWith("/shop/Seller-ID-123");
  });
});

describe("searchMyListingsAction", () => {
  const page = {
    items: [{ id: "l1", title: "Áo thun" }],
    nextCursor: "next-should-not-be-followed",
    total: 1,
  };

  it("Seller shop UI: search runs on the server with the typed query - PUBLISHED, one page of 20", async () => {
    vi.mocked(listMyListings).mockResolvedValue(page as never);
    const res = await searchMyListingsAction("  áo ");
    expect(listMyListings).toHaveBeenCalledTimes(1);
    expect(listMyListings).toHaveBeenCalledWith({
      statuses: [ListingStatus.PUBLISHED],
      query: "áo",
      pageSize: 20,
    });
    expect(res.ok).toBe(true);
    if (res.ok) expect(JSON.stringify(res.data)).toContain("Áo thun");
  });

  it("Seller shop UI: search runs on the server with the typed query - query is never logged", async () => {
    const spies = (["log", "info", "warn", "error", "debug"] as const).map(
      (m) => vi.spyOn(console, m).mockImplementation(() => {}),
    );
    vi.mocked(listMyListings).mockResolvedValueOnce(page as never);
    await searchMyListingsAction("secret-áo");
    vi.mocked(listMyListings).mockRejectedValueOnce(
      new ConnectError("down", Code.Unavailable),
    );
    await searchMyListingsAction("secret-áo");
    for (const spy of spies) {
      expect(JSON.stringify(spy.mock.calls)).not.toContain("secret-áo");
      spy.mockRestore();
    }
  });

  it("Seller shop UI: a search failure leaves the selection intact - failure becomes an error result", async () => {
    vi.mocked(listMyListings).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    expect(await searchMyListingsAction("áo")).toEqual({
      ok: false,
      error: "Không tải được danh sách sản phẩm.",
    });
  });

  it("Seller shop UI: search query over 100 characters is refused before any call", async () => {
    const res = await searchMyListingsAction("a".repeat(101));
    expect(res.ok).toBe(false);
    expect(listMyListings).not.toHaveBeenCalled();
  });

  it("searchMyListingsAction denies a principal without listing.write", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "buyer",
      name: "B",
      scopes: ["order.read"],
    });
    const res = await searchMyListingsAction("");
    expect(res.ok).toBe(false);
    expect(JSON.stringify(res)).not.toContain("Áo thun");
    expect(listMyListings).not.toHaveBeenCalled();
  });

  it("Search fails: the action returns an error result and no listing", async () => {
    vi.mocked(listMyListings).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    const res = await searchMyListingsAction("");
    expect(res.ok).toBe(false);
    expect("data" in res && res.data).toBeFalsy();
  });

  it("Seller shop UI: a principal without listing.write cannot search", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    expect((await searchMyListingsAction("áo")).ok).toBe(false);
    vi.mocked(getPrincipal).mockReturnValue({
      id: "s",
      name: "S",
      scopes: [],
    });
    expect((await searchMyListingsAction("áo")).ok).toBe(false);
    expect(listMyListings).not.toHaveBeenCalled();
  });
});

describe("marketing actions", () => {
  const voucherInput = {
    code: "GIAM10",
    discountType: DiscountType.PERCENT,
    discountValue: 10,
    minSpend: 0,
    maxDiscount: 0,
    quota: 100,
    startsAt: new Date("2026-06-16T00:00:00Z"),
    endsAt: new Date("2026-06-30T00:00:00Z"),
  };
  const campaignInput = {
    listingId: "l1",
    salePrice: 50000,
    stockCap: 10,
    startsAt: new Date("2026-06-16T00:00:00Z"),
    endsAt: new Date("2026-06-17T00:00:00Z"),
    idempotencyKey: "key-12345678",
  };

  it("marketing actions answer denied without listing.write", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "buyer",
      name: "B",
      scopes: [],
    });
    const results = [
      await createShopVoucherAction(voucherInput),
      await deactivateShopVoucherAction("v1"),
      await createCampaignAction(campaignInput),
      await cancelCampaignAction("c1"),
    ];
    for (const r of results) expect(r.ok).toBe(false);
    expect(createVoucher).not.toHaveBeenCalled();
    expect(deactivateMyShopVoucher).not.toHaveBeenCalled();
    expect(createCampaign).not.toHaveBeenCalled();
    expect(cancelMyCampaign).not.toHaveBeenCalled();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("marketing actions answer denied for an anonymous principal", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    expect((await deactivateShopVoucherAction("v1")).ok).toBe(false);
    expect(deactivateMyShopVoucher).not.toHaveBeenCalled();
  });

  it("createShopVoucherAction creates a SHOP voucher without a seller id and revalidates the hub", async () => {
    vi.mocked(createVoucher).mockResolvedValue({ id: "v1" } as never);
    const res = await createShopVoucherAction(voucherInput);
    expect(res.ok).toBe(true);
    expect(createVoucher).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(createVoucher).mock.calls[0][0] as unknown as Record<
      string,
      unknown
    >;
    expect(arg.scope).toBe(VoucherScope.SHOP);
    expect(arg.code).toBe("GIAM10");
    expect(arg).not.toHaveProperty("sellerId");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("Duplicate code shows the server error: ALREADY_EXISTS answers Mã voucher đã tồn tại", async () => {
    vi.mocked(createVoucher).mockRejectedValue(
      new ConnectError("dup", Code.AlreadyExists),
    );
    expect(await createShopVoucherAction(voucherInput)).toEqual({
      ok: false,
      error: "Mã voucher đã tồn tại",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("deactivateShopVoucherAction deactivates and revalidates the hub", async () => {
    vi.mocked(deactivateMyShopVoucher).mockResolvedValue({
      ok: true,
      voucher: { id: "v1" },
    } as never);
    expect((await deactivateShopVoucherAction("v1")).ok).toBe(true);
    expect(deactivateMyShopVoucher).toHaveBeenCalledWith("v1");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("deactivating an already deactivated voucher is accepted", async () => {
    vi.mocked(deactivateMyShopVoucher).mockResolvedValue({
      ok: true,
      voucher: { id: "v1", active: false },
    } as never);
    expect(await deactivateShopVoucherAction("v1")).toEqual({ ok: true });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("Another seller's voucher id is denied: message shown and nothing revalidated", async () => {
    vi.mocked(deactivateMyShopVoucher).mockResolvedValue({
      ok: false,
      code: "denied",
      message: "perm",
    } as never);
    expect(await deactivateShopVoucherAction("v-foreign")).toEqual({
      ok: false,
      error: "Không có quyền hoặc voucher không tồn tại",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("createCampaignAction forwards the input and revalidates the hub", async () => {
    vi.mocked(createCampaign).mockResolvedValue({
      ok: true,
      campaign: { id: "c1" },
    } as never);
    const res = await createCampaignAction(campaignInput);
    expect(res.ok).toBe(true);
    expect(createCampaign).toHaveBeenCalledWith(campaignInput);
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("createCampaignAction maps PERMISSION_DENIED for a foreign listing", async () => {
    vi.mocked(createCampaign).mockResolvedValue({
      ok: false,
      code: "denied",
      message: "perm",
    } as never);
    const res = await createCampaignAction(campaignInput);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.length).toBeGreaterThan(0);
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("createCampaignAction shows the server message for a rejected price", async () => {
    vi.mocked(createCampaign).mockResolvedValue({
      ok: false,
      code: "invalid",
      message: "sale price must be below listing price",
    } as never);
    expect(await createCampaignAction(campaignInput)).toEqual({
      ok: false,
      error: "sale price must be below listing price",
    });
  });

  it("cancelCampaignAction cancels and revalidates the hub", async () => {
    vi.mocked(cancelMyCampaign).mockResolvedValue({
      ok: true,
      campaign: { id: "c1" },
    } as never);
    expect((await cancelCampaignAction("c1")).ok).toBe(true);
    expect(cancelMyCampaign).toHaveBeenCalledWith("c1");
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("Cancelling a campaign that started meanwhile is refused: shows Flash sale đã bắt đầu, không thể huỷ and still revalidates", async () => {
    vi.mocked(cancelMyCampaign).mockResolvedValue({
      ok: false,
      code: "failed_precondition",
      message: "not scheduled",
    } as never);
    expect(await cancelCampaignAction("c1")).toEqual({
      ok: false,
      error: "Flash sale đã bắt đầu, không thể huỷ",
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/marketing");
  });

  it("Another seller's campaign id is denied: message shown and nothing revalidated", async () => {
    vi.mocked(cancelMyCampaign).mockResolvedValue({
      ok: false,
      code: "denied",
      message: "perm",
    } as never);
    expect(await cancelCampaignAction("c-foreign")).toEqual({
      ok: false,
      error: "Không có quyền hoặc flash sale không tồn tại",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("subscribeAction gateway refusals", () => {
  it("Insufficient balance is explained and nothing changes: FAILED_PRECONDITION", async () => {
    vi.mocked(subscribe).mockRejectedValue(
      new ConnectError("balance", Code.FailedPrecondition),
    );
    expect(await subscribeAction("p-pro")).toEqual({
      ok: false,
      error: "Số dư có thể rút không đủ để trả gói này.",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("Subscribe denied by the gateway shows an error and no change", async () => {
    for (const code of [Code.PermissionDenied, Code.Unauthenticated]) {
      vi.mocked(subscribe).mockRejectedValue(new ConnectError("x", code));
      expect(await subscribeAction("p-pro")).toEqual({
        ok: false,
        error: "Đăng ký gói thất bại.",
      });
    }
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("cancelSubscriptionAction", () => {
  it("Confirmed cancel calls CancelSubscription once", async () => {
    vi.mocked(cancelSubscription).mockResolvedValue({} as never);
    expect(await cancelSubscriptionAction()).toEqual({ ok: true });
    expect(cancelSubscription).toHaveBeenCalledTimes(1);
    expect(cancelSubscription).toHaveBeenCalledWith();
    expect(revalidatePath).toHaveBeenCalledWith("/seller/plans");
  });

  it("Cancel denied by the gateway shows an error", async () => {
    for (const code of [
      Code.PermissionDenied,
      Code.NotFound,
      Code.Unavailable,
    ]) {
      vi.mocked(cancelSubscription).mockRejectedValue(
        new ConnectError("x", code),
      );
      expect(await cancelSubscriptionAction()).toEqual({
        ok: false,
        error: "Hủy gia hạn thất bại.",
      });
    }
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
