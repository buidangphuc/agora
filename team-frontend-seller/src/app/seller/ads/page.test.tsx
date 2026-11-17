import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { AdCampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { getListing } from "@/lib/gateway/listings";
import {
  type ViewAdCampaign,
  listMyAdCampaigns,
} from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerAdsPage from "./page";

vi.mock("next/navigation", () => ({
  redirect: vi.fn(),
  useRouter: () => ({ refresh: vi.fn() }),
  usePathname: () => "/seller/ads",
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("guest-redirect");
  }),
}));
vi.mock("@/features/seller/actions", () => ({
  createAdCampaignAction: vi.fn(),
  pauseAdCampaignAction: vi.fn(),
  resumeAdCampaignAction: vi.fn(),
  searchMyListingsAction: vi.fn(() => new Promise(() => {})),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getListing: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", async (orig) => ({
  ...(await orig<typeof import("@/lib/gateway/promotion")>()),
  listMyAdCampaigns: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));

function campaigns(n: number): ViewAdCampaign[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `c${i + 1}`,
    sellerId: "s1",
    listingId: "l1",
    budget: 50000,
    bid: 1000,
    spent: 0,
    impressions: 0,
    clicks: 0,
    status: AdCampaignStatus.ACTIVE,
    statusText: "Đang chạy",
    createdAt: "",
  }));
}

async function renderPage(searchParams?: Record<string, string>) {
  const ui = await SellerAdsPage({
    searchParams: Promise.resolve(searchParams ?? {}),
  } as never);
  render(ui as never);
}

function dataRows() {
  return screen.getAllByRole("button", { name: "Tạm dừng" }).length;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "s1",
    name: "Seller",
    scopes: ["listing.write"],
  } as never);
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(getListing).mockResolvedValue({
    id: "l1",
    title: "Áo thun",
  } as never);
});

describe("/seller/ads page", () => {
  it("pages campaigns at 20", async () => {
    vi.mocked(listMyAdCampaigns).mockResolvedValue({
      ok: true,
      campaigns: campaigns(21),
    });
    await renderPage();
    expect(dataRows()).toBe(20);
    expect(screen.getAllByText("Áo thun").length).toBeGreaterThanOrEqual(20);
  });

  it("pages campaigns at 20: page 2 shows the remaining row", async () => {
    vi.mocked(listMyAdCampaigns).mockResolvedValue({
      ok: true,
      campaigns: campaigns(21),
    });
    await renderPage({ page: "2" });
    expect(dataRows()).toBe(1);
  });

  it("list error degrades only the list", async () => {
    vi.mocked(listMyAdCampaigns).mockResolvedValue({ ok: false });
    await renderPage();
    expect(screen.getByText("Không tải được chiến dịch")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Thử lại" })).toBeInTheDocument();
    expect(screen.getByText("Tạo chiến dịch mới")).toBeInTheDocument();
  });

  it("empty list still renders the create form", async () => {
    vi.mocked(listMyAdCampaigns).mockResolvedValue({
      ok: true,
      campaigns: [],
    });
    await renderPage();
    expect(screen.getByText("Chưa có chiến dịch nào")).toBeInTheDocument();
    expect(screen.getByText("Tạo chiến dịch mới")).toBeInTheDocument();
  });

  it("guest is redirected to login and no list call is made", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(renderPage()).rejects.toThrow("guest-redirect");
    expect(redirectGuestToLogin).toHaveBeenCalledWith(
      "/seller/ads",
      expect.anything(),
    );
    expect(listMyAdCampaigns).not.toHaveBeenCalled();
  });

  it("non-seller without listing.write sees no data and no list call is made", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    const ui = await SellerAdsPage({
      searchParams: Promise.resolve({}),
    } as never);
    expect(ui).toBeNull();
    expect(listMyAdCampaigns).not.toHaveBeenCalled();
  });
});
