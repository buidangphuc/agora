import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { cancelCampaignAction } from "../actions";
import { FlashSaleTab } from "./FlashSaleTab";
import { campaignRow } from "./testRows";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
  usePathname: () => "/seller/marketing",
  useSearchParams: () => new URLSearchParams(),
}));
vi.mock("../actions", () => ({
  createCampaignAction: vi.fn(),
  cancelCampaignAction: vi.fn(),
  searchMyListingsAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(cancelCampaignAction).mockResolvedValue({ ok: true });
});

const rowFor = (text: string) =>
  screen.getByText(text).closest('[data-testid="campaign-row"]') as HTMLElement;

describe("FlashSaleTab", () => {
  it("Campaigns are listed with status and sold over cap: title, status and sold/cap per row", () => {
    render(
      <FlashSaleTab
        titles={{ "lst-1": "Áo thun", "lst-2": "Giày" }}
        rows={[
          campaignRow({ id: "c1", listingId: "lst-1", stockCap: 10 }),
          campaignRow({
            id: "c2",
            listingId: "lst-2",
            status: CampaignStatus.ACTIVE,
            stockCap: 20,
            stockSold: 5,
          }),
        ]}
      />,
    );
    expect(within(rowFor("Áo thun")).getByText("Sắp diễn ra")).toBeInTheDocument();
    expect(within(rowFor("Áo thun")).getByText("0/10")).toBeInTheDocument();
    expect(within(rowFor("Giày")).getByText("Đang diễn ra")).toBeInTheDocument();
    expect(within(rowFor("Giày")).getByText("5/20")).toBeInTheDocument();
  });

  it("keeps the row with the listing id when the title lookup fails", () => {
    render(
      <FlashSaleTab
        titles={{}}
        rows={[campaignRow({ id: "c1", listingId: "lst-unknown" })]}
      />,
    );
    expect(screen.getAllByTestId("campaign-row")).toHaveLength(1);
    expect(within(rowFor("lst-unknown")).getByText("0/10")).toBeInTheDocument();
  });

  it("Cancelling a scheduled campaign: confirming calls the action with only the campaign id", async () => {
    render(
      <FlashSaleTab
        titles={{ "lst-1": "Áo thun" }}
        rows={[campaignRow({ id: "c7", listingId: "lst-1" })]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Huỷ" }));
    expect(cancelCampaignAction).not.toHaveBeenCalled();
    fireEvent.click(await screen.findByRole("button", { name: "Xác nhận" }));
    await vi.waitFor(() =>
      expect(cancelCampaignAction).toHaveBeenCalledTimes(1),
    );
    expect(cancelCampaignAction).toHaveBeenCalledWith("c7");
  });

  it("Cancelling the confirmation changes nothing: Huỷ bỏ on a campaign calls nothing", async () => {
    render(
      <FlashSaleTab
        titles={{ "lst-1": "Áo thun" }}
        rows={[campaignRow({ id: "c7", listingId: "lst-1" })]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Huỷ" }));
    fireEvent.click(await screen.findByRole("button", { name: "Huỷ bỏ" }));
    expect(cancelCampaignAction).not.toHaveBeenCalled();
  });

  it("Cancelling a scheduled campaign: a server refusal (already started) is shown", async () => {
    vi.mocked(cancelCampaignAction).mockResolvedValue({
      ok: false,
      error: "Flash sale đã bắt đầu, không thể huỷ",
    });
    render(<FlashSaleTab titles={{}} rows={[campaignRow({ id: "c7" })]} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ" }));
    fireEvent.click(await screen.findByRole("button", { name: "Xác nhận" }));
    expect(
      await screen.findByText("Flash sale đã bắt đầu, không thể huỷ"),
    ).toBeInTheDocument();
  });

  it("Active and ended campaigns offer no cancel control: ACTIVE, ENDED, CANCELLED rows have no Huỷ", () => {
    render(
      <FlashSaleTab
        titles={{}}
        rows={[
          campaignRow({ id: "a", listingId: "la", status: CampaignStatus.ACTIVE }),
          campaignRow({ id: "e", listingId: "le", status: CampaignStatus.ENDED }),
          campaignRow({
            id: "x",
            listingId: "lx",
            status: CampaignStatus.CANCELLED,
          }),
        ]}
      />,
    );
    expect(screen.getAllByTestId("campaign-row")).toHaveLength(3);
    expect(screen.queryByRole("button", { name: "Huỷ" })).toBeNull();
  });

  it("Empty list shows an empty state: Tạo flash sale call to action and no alert", () => {
    render(<FlashSaleTab titles={{}} rows={[]} />);
    expect(screen.queryAllByTestId("campaign-row")).toHaveLength(0);
    expect(screen.getAllByText("Tạo flash sale").length).toBeGreaterThan(0);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("375 px layout: the row list scrolls or stacks instead of overflowing the viewport", () => {
    render(<FlashSaleTab titles={{}} rows={[campaignRow()]} />);
    expect(screen.getByTestId("marketing-list").className).toMatch(
      /overflow-x-auto|flex-col|grid-cols-1|space-y/,
    );
  });
});
