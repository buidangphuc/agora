import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { campaignRow, voucherRow } from "@/features/seller/marketing/testRows";
import { CampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { getListing, listMyListings } from "@/lib/gateway/listings";
import { listMyCampaigns, listMyShopVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import MarketingPage from "./page";

class GuestRedirect extends Error {}

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  notFound: vi.fn(),
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
  usePathname: () => "/seller/marketing",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new GuestRedirect("guest-redirect");
  }),
  redirectGuestToLoginFromHeader: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", async (orig) => ({
  ...(await orig<typeof import("@/lib/gateway/promotion")>()),
  listMyShopVouchers: vi.fn(),
  listMyCampaigns: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", async (orig) => ({
  ...(await orig<typeof import("@/lib/gateway/listings")>()),
  getListing: vi.fn(),
  listMyListings: vi.fn(),
}));
vi.mock("@/features/seller/actions", () => ({
  createShopVoucherAction: vi.fn(),
  deactivateShopVoucherAction: vi.fn(),
  createCampaignAction: vi.fn(),
  cancelCampaignAction: vi.fn(),
  searchMyListingsAction: vi.fn(),
}));

const seller = { id: "seller-a", name: "A", scopes: ["listing.write"] };
const buyer = { id: "buyer-1", name: "B", scopes: [] };

const vouchersOk = (n: number, nextCursor = "") =>
  ({
    ok: true,
    items: Array.from({ length: n }, (_, i) =>
      voucherRow({ id: `v${i}`, code: `CODE${i}` }),
    ),
    nextCursor,
  }) as never;

async function renderPage(sp: Record<string, string> = {}) {
  render(await MarketingPage({ searchParams: Promise.resolve(sp) }));
}

const isSelected = (name: string) =>
  screen
    .getAllByText(name)
    .some((el) =>
      el.closest('[aria-selected="true"],[aria-current="page"],[aria-current="true"]'),
    );

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(seller as never);
  vi.mocked(hasScope).mockImplementation((s: string) =>
    Boolean(vi.mocked(getPrincipal)()?.scopes.includes(s)),
  );
  vi.mocked(listMyShopVouchers).mockResolvedValue(vouchersOk(1));
  vi.mocked(listMyCampaigns).mockResolvedValue({
    ok: true,
    items: [],
    nextCursor: "",
  });
  vi.mocked(getListing).mockRejectedValue(new Error("listing unavailable"));
  vi.mocked(listMyListings).mockRejectedValue(new Error("listing unavailable"));
});

describe("/seller/marketing page", () => {
  it("Seller opens the hub from the navigation: voucher tab is selected by default and lists vouchers", async () => {
    await renderPage();
    expect(isSelected("Voucher của shop")).toBe(true);
    expect(screen.getAllByTestId("voucher-row")).toHaveLength(1);
    expect(listMyCampaigns).not.toHaveBeenCalled();
  });

  it("Tab is kept in the URL: tab links carry tab= and the flashsale tab is selected for ?tab=flashsale", async () => {
    await renderPage({ tab: "flashsale" });
    expect(isSelected("Flash sale")).toBe(true);
    expect(isSelected("Voucher của shop")).toBe(false);
    expect(
      screen.getByRole("link", { name: "Voucher của shop" }).getAttribute("href"),
    ).toContain("tab=voucher");
    expect(listMyCampaigns).toHaveBeenCalledTimes(1);
    expect(listMyShopVouchers).not.toHaveBeenCalled();
  });

  it("falls back to the voucher tab for an unknown tab value", async () => {
    await renderPage({ tab: "bogus" });
    expect(isSelected("Voucher của shop")).toBe(true);
    expect(screen.queryByText("Thử lại")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(listMyShopVouchers).toHaveBeenCalledTimes(1);
  });

  it("Buyer account is denied the hub: renders nothing and calls no promotion RPC", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer as never);
    const result = await MarketingPage({ searchParams: Promise.resolve({}) });
    expect(result).toBeNull();
    expect(listMyShopVouchers).not.toHaveBeenCalled();
    expect(listMyCampaigns).not.toHaveBeenCalled();
    expect(redirectGuestToLogin).not.toHaveBeenCalled();
  });

  it("does not call ListMyShopVouchers without listing.write", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer as never);
    await MarketingPage({ searchParams: Promise.resolve({ tab: "voucher" }) });
    expect(listMyShopVouchers).not.toHaveBeenCalled();
  });

  it("does not call ListMyCampaigns without listing.write", async () => {
    vi.mocked(getPrincipal).mockReturnValue(buyer as never);
    await MarketingPage({ searchParams: Promise.resolve({ tab: "flashsale" }) });
    expect(listMyCampaigns).not.toHaveBeenCalled();
  });

  it("Anonymous visitor is redirected to login: with the page's own path", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(
      MarketingPage({ searchParams: Promise.resolve({ tab: "flashsale" }) }),
    ).rejects.toBeInstanceOf(GuestRedirect);
    expect(vi.mocked(redirectGuestToLogin).mock.calls[0][0]).toBe(
      "/seller/marketing",
    );
    expect(listMyShopVouchers).not.toHaveBeenCalled();
  });

  it("keeps the other tab usable when ListMyCampaigns is unavailable", async () => {
    vi.mocked(listMyCampaigns).mockResolvedValue({
      ok: false,
      code: "unavailable",
      message: "down",
    });
    await renderPage({ tab: "flashsale" });
    expect(screen.getByText("Thử lại")).toBeInTheDocument();
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
    const voucherLink = screen.getByRole("link", { name: "Voucher của shop" });
    expect(voucherLink).toBeInTheDocument();
    expect(voucherLink.getAttribute("href")).toContain("tab=voucher");

    document.body.innerHTML = "";
    await renderPage({ tab: "voucher" });
    expect(screen.getAllByTestId("voucher-row")).toHaveLength(1);
    expect(screen.queryByText("Thử lại")).toBeNull();
  });

  it("shows the retry state for a malformed cursor", async () => {
    vi.mocked(listMyShopVouchers).mockResolvedValue({
      ok: false,
      code: "invalid",
      message: "bad cursor",
    });
    await renderPage({ cursor: "!!bad" });
    expect(vi.mocked(listMyShopVouchers).mock.calls[0][0]).toMatchObject({
      cursor: "!!bad",
    });
    expect(screen.getByText("Thử lại")).toBeInTheDocument();
    expect(screen.queryAllByTestId("voucher-row")).toHaveLength(0);
  });

  it("Paging boundary at 21 vouchers: 20 rows with a next link, then 1 row without one", async () => {
    vi.mocked(listMyShopVouchers).mockResolvedValueOnce(vouchersOk(20, "c20"));
    const { container } = render(
      await MarketingPage({ searchParams: Promise.resolve({}) }),
    );
    expect(screen.getAllByTestId("voucher-row")).toHaveLength(20);
    const next = container.querySelector('a[href*="cursor=c20"]');
    expect(next).not.toBeNull();
    expect(next?.getAttribute("href")).toContain("tab=voucher");
    expect(vi.mocked(listMyShopVouchers).mock.calls[0][0].cursor ?? "").toBe("");

    document.body.innerHTML = "";
    vi.mocked(listMyShopVouchers).mockResolvedValueOnce(vouchersOk(1, ""));
    const second = render(
      await MarketingPage({
        searchParams: Promise.resolve({ cursor: "c20" }),
      }),
    );
    expect(vi.mocked(listMyShopVouchers).mock.calls[1][0]).toMatchObject({
      cursor: "c20",
    });
    expect(screen.getAllByTestId("voucher-row")).toHaveLength(1);
    expect(second.container.querySelector('a[rel="next"]')).toBeNull();
  });

  it("Empty list shows an empty state: vouchers", async () => {
    vi.mocked(listMyShopVouchers).mockResolvedValue(vouchersOk(0));
    await renderPage();
    expect(screen.queryAllByTestId("voucher-row")).toHaveLength(0);
    expect(screen.getAllByText("Tạo voucher").length).toBeGreaterThan(0);
    expect(screen.queryByText("Thử lại")).toBeNull();
  });

  it("Empty list shows an empty state: campaigns", async () => {
    await renderPage({ tab: "flashsale" });
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
    expect(screen.getAllByText("Tạo flash sale").length).toBeGreaterThan(0);
    expect(screen.queryByText("Thử lại")).toBeNull();
  });

  it("Hub status filter narrows the list: status=cancelled is sent as statuses=[CANCELLED]", async () => {
    vi.mocked(listMyCampaigns).mockResolvedValue({
      ok: true,
      items: [campaignRow({ id: "x", status: CampaignStatus.CANCELLED })],
      nextCursor: "",
    });
    await renderPage({ tab: "flashsale", status: "cancelled" });
    expect(vi.mocked(listMyCampaigns).mock.calls[0][0].statuses).toEqual([
      CampaignStatus.CANCELLED,
    ]);
    const filter = screen.getByRole("link", { name: "Đã huỷ" });
    expect(filter.getAttribute("href")).toContain("status=cancelled");
    expect(filter.getAttribute("href")).toContain("tab=flashsale");
    expect(screen.getAllByTestId("campaign-row")).toHaveLength(1);
  });

  it("Hub status filter narrows the list: no status sends no status filter", async () => {
    await renderPage({ tab: "flashsale" });
    expect(vi.mocked(listMyCampaigns).mock.calls[0][0].statuses ?? []).toEqual([]);
  });

  it("Listing title lookup failure does not hide the row: the listing id stands in for the title", async () => {
    vi.mocked(listMyCampaigns).mockResolvedValue({
      ok: true,
      items: [campaignRow({ id: "c1", listingId: "lst-77" })],
      nextCursor: "",
    });
    await renderPage({ tab: "flashsale" });
    const row = screen.getByTestId("campaign-row");
    expect(within(row).getByText(/lst-77/)).toBeInTheDocument();
  });
});
