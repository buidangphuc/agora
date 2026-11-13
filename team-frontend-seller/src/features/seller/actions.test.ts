import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import {
  createBundle,
  getStorefront,
  listMyListings,
  upsertStorefront,
} from "@/lib/gateway/listings";
import { requestWalletPayout } from "@/lib/gateway/payment";
import { createAdCampaign, subscribe } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import {
  createAdCampaignAction,
  createBundleAction,
  requestWalletPayoutAction,
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
  subscribe: vi.fn(),
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
    expect(await createAdCampaignAction("l", 100, 5)).toEqual({
      ok: true,
      data: { campaign },
    });
    expect(revalidatePath).toHaveBeenCalledWith("/seller/ads");
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
