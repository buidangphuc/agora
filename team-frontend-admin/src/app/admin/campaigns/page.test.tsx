import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { type CampaignView, listCampaigns } from "@/lib/gateway/campaigns";
import { getPrincipal, getToken } from "@/lib/gateway/session";

import CampaignsPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/campaigns", () => ({ listCampaigns: vi.fn() }));
vi.mock("@/features/campaigns/actions", () => ({
  cancelCampaignAction: vi.fn(),
  updateCampaignAction: vi.fn(),
  createCampaignAction: vi.fn(),
}));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

const campaign: CampaignView = {
  id: "fs-1",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 7,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
  status: "active",
  cancelledAt: "",
};

const page = (sp: Record<string, string> = {}) =>
  CampaignsPage({ searchParams: Promise.resolve(sp) });

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listCampaigns).mockResolvedValue({
    status: "ok",
    campaigns: [campaign],
    nextCursor: "",
    flashSaleDisabled: false,
  });
});

describe("CampaignsPage", () => {
  it("declares no-referrer metadata", () => {
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("forbids non-admin: a seller sees 403 and no campaign data is requested", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-2",
      name: "Seller",
      scopes: ["listing.write"],
    });
    render(await page());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
    expect(listCampaigns).not.toHaveBeenCalled();
  });

  it("forbids non-admin: an anonymous visitor is redirected to login and nothing is requested", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await page()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/campaigns")}`,
    );
    expect(listCampaigns).not.toHaveBeenCalled();
  });

  it("lists every campaign with no filter and the first page", async () => {
    render(await page());
    expect(listCampaigns).toHaveBeenCalledWith([], "");
    expect(screen.getAllByTestId("campaign-row")).toHaveLength(1);
  });

  it("filter and cancel from the list: the status param becomes the request filter", async () => {
    render(await page({ status: "active", cursor: "c1" }));
    expect(listCampaigns).toHaveBeenCalledWith(["active"], "c1");
    expect(screen.getByTestId("campaign-filter-active")).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("treats an unknown status param as no filter", async () => {
    render(await page({ status: "bogus" }));
    expect(listCampaigns).toHaveBeenCalledWith([], "");
  });

  it("shows kill-switch banner instead of the empty message", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      status: "ok",
      campaigns: [],
      nextCursor: "",
      flashSaleDisabled: true,
    });
    render(await page());
    expect(screen.getByTestId("flash-sale-disabled-banner")).toHaveTextContent(
      /flash sale is switched off/i,
    );
    expect(screen.queryByTestId("campaigns-empty")).toBeNull();
  });

  it("shows the empty message when there are no campaigns and the switch is on", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({
      status: "ok",
      campaigns: [],
      nextCursor: "",
      flashSaleDisabled: false,
    });
    render(await page());
    expect(screen.getByTestId("campaigns-empty")).toBeInTheDocument();
  });

  it("shows 403 when the gateway refuses the admin scope", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({ status: "forbidden" });
    render(await page());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
  });

  it("redirects to login when the gateway reports an expired session", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({ status: "unauthenticated" });
    expect(await page()).toBeNull();
    expect(redirect).toHaveBeenCalled();
  });

  it("shows an error alert when the service is down", async () => {
    vi.mocked(listCampaigns).mockResolvedValue({ status: "unavailable" });
    render(await page());
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.queryByTestId("campaigns-empty")).toBeNull();
  });
});
