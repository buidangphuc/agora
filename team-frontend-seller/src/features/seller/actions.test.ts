import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createBundle,
  getStorefront,
  upsertStorefront,
} from "@/lib/gateway/listings";
import { requestWalletPayout } from "@/lib/gateway/payment";
import { createAdCampaign, subscribe } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import {
  createAdCampaignAction,
  createBundleAction,
  requestWalletPayoutAction,
  subscribeAction,
  upsertStorefrontAction,
} from "./actions";

vi.mock("@/lib/gateway/listings", () => ({
  createBundle: vi.fn(),
  getStorefront: vi.fn(),
  upsertStorefront: vi.fn(),
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

describe("upsertStorefrontAction", () => {
  it("rejects blank and over-long names before any gateway call", async () => {
    expect(await upsertStorefrontAction("   ")).toEqual({
      ok: false,
      error: "Nhập tên gian hàng.",
    });
    expect(await upsertStorefrontAction("x".repeat(81))).toMatchObject({
      ok: false,
      error: "Tên gian hàng tối đa 80 ký tự.",
    });
    expect(getStorefront).not.toHaveBeenCalled();
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("keeps the rest of the existing storefront and trims the name", async () => {
    vi.mocked(getStorefront).mockResolvedValue({
      sellerId: "Seller-ID-123",
      slug: "tiem-hoa",
      bannerUrl: "b.png",
      tagline: "Hoa tươi",
      featuredListingIds: ["l1"],
      theme: "warm",
      displayName: "Cũ",
    });
    vi.mocked(upsertStorefront).mockResolvedValue({
      displayName: "Nhà Sách An Nhiên",
    } as never);
    const res = await upsertStorefrontAction("  Nhà Sách An Nhiên  ");
    expect(res).toEqual({
      ok: true,
      data: { displayName: "Nhà Sách An Nhiên" },
    });
    expect(getStorefront).toHaveBeenCalledWith("Seller-ID-123", {
      throwOnError: true,
    });
    expect(upsertStorefront).toHaveBeenCalledWith({
      sellerId: "Seller-ID-123",
      slug: "tiem-hoa",
      bannerUrl: "b.png",
      tagline: "Hoa tươi",
      featuredListingIds: ["l1"],
      theme: "warm",
      displayName: "Nhà Sách An Nhiên",
    });
    expect(revalidatePath).toHaveBeenCalledWith("/shop/Seller-ID-123");
  });

  it("derives the required slug from the session when there is no storefront", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    vi.mocked(upsertStorefront).mockResolvedValue({
      displayName: "A",
    } as never);
    await upsertStorefrontAction("A");
    expect(upsertStorefront).toHaveBeenCalledWith({
      sellerId: "Seller-ID-123",
      slug: "shop-sellerid123",
      displayName: "A",
    });
  });

  it("never overwrites the storefront when the current one cannot be read", async () => {
    vi.mocked(getStorefront).mockRejectedValue(
      new ConnectError("down", Code.Unavailable),
    );
    const res = await upsertStorefrontAction("A");
    expect(res.ok).toBe(false);
    expect(upsertStorefront).not.toHaveBeenCalled();
  });

  it("returns the gateway error", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    vi.mocked(upsertStorefront).mockRejectedValue(new Error("slug taken"));
    expect(await upsertStorefrontAction("A")).toEqual({
      ok: false,
      error: "slug taken",
    });
  });

  it("requires a signed-in seller", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    expect((await upsertStorefrontAction("A")).ok).toBe(false);
    expect(upsertStorefront).not.toHaveBeenCalled();
  });
});
