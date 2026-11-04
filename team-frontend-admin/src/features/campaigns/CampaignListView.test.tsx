import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { CampaignView } from "@/lib/gateway/campaigns";

import { CampaignListView } from "./CampaignListView";

vi.mock("./actions", () => ({
  cancelCampaignAction: vi.fn(),
  updateCampaignAction: vi.fn(),
  createCampaignAction: vi.fn(),
}));

const campaign = (
  id: string,
  status: CampaignView["status"],
): CampaignView => ({
  id,
  listingId: `l-${id}`,
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 0,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
  status,
  cancelledAt: "",
});

const props = {
  campaigns: [] as CampaignView[],
  nextCursor: "",
  cursor: "",
  filter: "all" as const,
  flashSaleDisabled: false,
};

const hrefOf = (testId: string) =>
  new URL(
    screen.getByTestId(testId).getAttribute("href") ?? "",
    "http://x.test",
  );

describe("CampaignListView", () => {
  it("shows kill-switch banner and not the empty-list message", () => {
    render(<CampaignListView {...props} flashSaleDisabled />);
    expect(screen.getByTestId("flash-sale-disabled-banner")).toHaveTextContent(
      /flash sale is switched off/i,
    );
    expect(screen.queryByTestId("campaigns-empty")).toBeNull();
  });

  it("shows kill-switch banner while still listing the returned campaigns", () => {
    render(
      <CampaignListView
        {...props}
        flashSaleDisabled
        campaigns={[campaign("a", "scheduled")]}
      />,
    );
    expect(
      screen.getByTestId("flash-sale-disabled-banner"),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId("campaign-row")).toHaveLength(1);
  });

  it("shows the empty message and no banner when the switch is on", () => {
    render(<CampaignListView {...props} />);
    expect(screen.getByTestId("campaigns-empty")).toBeInTheDocument();
    expect(screen.queryByTestId("flash-sale-disabled-banner")).toBeNull();
  });

  it("renders one row per campaign with its status", () => {
    render(
      <CampaignListView
        {...props}
        campaigns={[campaign("a", "active"), campaign("b", "ended")]}
      />,
    );
    const rows = screen.getAllByTestId("campaign-row");
    expect(rows).toHaveLength(2);
    expect(
      within(rows[0])
        .getByTestId("campaign-status")
        .getAttribute("data-status"),
    ).toBe("active");
    expect(
      within(rows[1])
        .getByTestId("campaign-status")
        .getAttribute("data-status"),
    ).toBe("ended");
  });

  it("filter and cancel from the list: filter links cover all, scheduled, active, ended, cancelled", () => {
    render(<CampaignListView {...props} filter="active" />);
    expect(hrefOf("campaign-filter-all").pathname).toBe("/admin/campaigns");
    expect(hrefOf("campaign-filter-all").searchParams.get("status")).toBeNull();
    for (const s of ["scheduled", "active", "ended", "cancelled"]) {
      const url = hrefOf(`campaign-filter-${s}`);
      expect(url.pathname).toBe("/admin/campaigns");
      expect(url.searchParams.get("status")).toBe(s);
      expect(url.searchParams.get("cursor")).toBeNull();
    }
    expect(screen.getByTestId("campaign-filter-active")).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(screen.getByTestId("campaign-filter-all")).not.toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("pages forward keeping the filter and back to the first page", () => {
    render(
      <CampaignListView
        {...props}
        filter="ended"
        cursor="c1"
        nextCursor="c2"
        campaigns={[campaign("a", "ended")]}
      />,
    );
    const next = hrefOf("campaigns-next");
    expect(next.pathname).toBe("/admin/campaigns");
    expect(next.searchParams.get("cursor")).toBe("c2");
    expect(next.searchParams.get("status")).toBe("ended");
    const first = hrefOf("campaigns-first");
    expect(first.searchParams.get("cursor")).toBeNull();
    expect(first.searchParams.get("status")).toBe("ended");
  });

  it("hides paging links on a single page", () => {
    render(
      <CampaignListView {...props} campaigns={[campaign("a", "active")]} />,
    );
    expect(screen.queryByTestId("campaigns-next")).toBeNull();
    expect(screen.queryByTestId("campaigns-first")).toBeNull();
  });

  it("links to the create form", () => {
    render(<CampaignListView {...props} />);
    expect(hrefOf("campaign-create-link").pathname).toBe(
      "/admin/campaigns/new",
    );
  });

  it("shows a load error instead of rows or the empty message", () => {
    render(<CampaignListView {...props} error="unavailable" />);
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.queryByTestId("campaigns-empty")).toBeNull();
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
  });
});
