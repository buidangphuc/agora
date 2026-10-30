import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CampaignsView } from "./CampaignsView";

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

describe("CampaignsView", () => {
  it("lists active campaigns with stock and a create link", () => {
    render(
      <CampaignsView campaigns={[campaign]} nextCursor="" lookup={null} />,
    );
    const rows = screen.getAllByTestId("campaigns-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("fs-1");
    expect(rows[0]).toHaveTextContent("7 / 50");
    expect(
      screen.getByRole("link", { name: "Tạo flash sale" }),
    ).toHaveAttribute("href", "/admin/campaigns/new");
    expect(screen.getByTestId("campaigns-gap")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("shows the empty state", () => {
    render(<CampaignsView campaigns={[]} nextCursor="" lookup={null} />);
    expect(
      screen.getByText("Không có chiến dịch nào đang chạy."),
    ).toBeInTheDocument();
  });

  it("shows the load error", () => {
    render(
      <CampaignsView
        campaigns={[]}
        nextCursor=""
        lookup={null}
        error="unavailable"
      />,
    );
    expect(
      screen.getByText(
        "Không tải được danh sách chiến dịch. Vui lòng thử lại sau.",
      ),
    ).toBeInTheDocument();
  });

  it("links the next page and keeps the lookup", () => {
    render(
      <CampaignsView
        campaigns={[campaign]}
        nextCursor="20"
        lookup={{ listingId: "l-9", status: "none" }}
      />,
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/campaigns?cursor=20&listing=l-9",
    );
  });

  it.each([
    [
      { listingId: "l-9", status: "none" } as const,
      "Tin l-9 không có flash sale nào đang chạy.",
    ],
    [
      { listingId: "l-9", status: "invalid" } as const,
      "Mã tin đăng không hợp lệ.",
    ],
    [
      { listingId: "l-9", status: "unavailable" } as const,
      "Không tra cứu được flash sale. Vui lòng thử lại sau.",
    ],
  ])("renders lookup %j", (lookup, text) => {
    render(<CampaignsView campaigns={[]} nextCursor="" lookup={lookup} />);
    expect(screen.getByTestId("campaign-lookup-result")).toHaveTextContent(
      text,
    );
    expect(screen.getByLabelText(/Tra cứu theo mã tin đăng/)).toHaveValue(
      "l-9",
    );
  });

  it("renders the active campaign of a looked-up listing", () => {
    render(
      <CampaignsView
        campaigns={[]}
        nextCursor=""
        lookup={{ listingId: "l-1", status: "active", campaign }}
      />,
    );
    expect(screen.getByTestId("campaign-lookup-result")).toHaveTextContent(
      "fs-1",
    );
  });
});
