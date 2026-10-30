import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  getActiveFlashSale,
  listActiveCampaigns,
} from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

import CampaignsPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/campaigns", () => ({
  listActiveCampaigns: vi.fn(),
  getActiveFlashSale: vi.fn(),
}));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };
const campaign = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 7,
  startsAt: "2026-11-01T01:00:00.000Z",
  endsAt: "2026-11-01T05:00:00.000Z",
};

const page = (sp: Record<string, string> = {}) =>
  CampaignsPage({ searchParams: Promise.resolve(sp) });

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listActiveCampaigns).mockResolvedValue({
    status: "ok",
    campaigns: [campaign],
    nextCursor: "",
  });
  vi.mocked(getActiveFlashSale).mockResolvedValue({
    status: "ok",
    active: false,
  });
});

describe("CampaignsPage", () => {
  it("declares no-referrer metadata", () => {
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await page()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/campaigns")}`,
    );
    expect(listActiveCampaigns).not.toHaveBeenCalled();
  });

  it("shows 403 to a non-admin without calling the gateway", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-2",
      name: "Seller",
      scopes: ["listing.write"],
    });
    render(await page());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(listActiveCampaigns).not.toHaveBeenCalled();
  });

  it("renders the active list and skips the lookup without a listing", async () => {
    render(await page({ cursor: "20" }));
    expect(listActiveCampaigns).toHaveBeenCalledWith("20");
    expect(getActiveFlashSale).not.toHaveBeenCalled();
    expect(screen.getAllByTestId("campaigns-row")).toHaveLength(1);
  });

  it("looks up the active sale of a listing", async () => {
    vi.mocked(getActiveFlashSale).mockResolvedValue({
      status: "ok",
      active: true,
      campaign,
    });
    render(await page({ listing: " l-1 " }));
    expect(getActiveFlashSale).toHaveBeenCalledWith("l-1");
    expect(screen.getByTestId("campaign-lookup-result")).toHaveTextContent(
      "fs-1",
    );
  });

  it("shows the gateway refusal as 403", async () => {
    vi.mocked(listActiveCampaigns).mockResolvedValue({ status: "forbidden" });
    render(await page());
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
  });

  it("shows the load error when the service is down", async () => {
    vi.mocked(listActiveCampaigns).mockResolvedValue({ status: "unavailable" });
    render(await page());
    expect(
      screen.getByText(
        "Không tải được danh sách chiến dịch. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
  });
});
