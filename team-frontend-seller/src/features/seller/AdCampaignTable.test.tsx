import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AdCampaignStatus } from "@/generated/platform/promotion/v1/promotion_pb.js";
import type { ViewAdCampaign } from "@/lib/gateway/promotion";
import { AdCampaignTable } from "./AdCampaignTable";
import { pauseAdCampaignAction, resumeAdCampaignAction } from "./actions";

vi.mock("./actions", () => ({
  pauseAdCampaignAction: vi.fn(),
  resumeAdCampaignAction: vi.fn(),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

const TEXT: Record<number, string> = {
  [AdCampaignStatus.ACTIVE]: "Đang chạy",
  [AdCampaignStatus.PAUSED]: "Tạm dừng",
  [AdCampaignStatus.PAUSED_BUDGET]: "Hết ngân sách",
  [AdCampaignStatus.ENDED]: "Đã kết thúc",
};

function row(
  status: AdCampaignStatus,
  over: Partial<ViewAdCampaign> = {},
): ViewAdCampaign & { listingTitle: string } {
  return {
    id: "c1",
    sellerId: "s1",
    listingId: "l1",
    listingTitle: "Áo thun",
    budget: 50000,
    bid: 1000,
    spent: 0,
    impressions: 0,
    clicks: 0,
    status,
    statusText: TEXT[status],
    createdAt: "",
    ...over,
  };
}

beforeEach(() => vi.clearAllMocks());

describe("AdCampaignTable", () => {
  it("renders empty state", () => {
    render(<AdCampaignTable campaigns={[]} />);
    expect(screen.getByText("Chưa có chiến dịch nào")).toBeInTheDocument();
  });

  it("renders title, money columns, counters and status for a row", () => {
    render(
      <AdCampaignTable
        campaigns={[
          row(AdCampaignStatus.ACTIVE, {
            spent: 1000,
            clicks: 1,
            impressions: 3,
          }),
        ]}
      />,
    );
    expect(screen.getByText("Áo thun")).toBeInTheDocument();
    expect(screen.getByText(/₫49\.000/)).toBeInTheDocument();
    expect(screen.getByText("Đang chạy")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tạm dừng" })).toBeEnabled();
  });

  it("ended row has no action", () => {
    render(<AdCampaignTable campaigns={[row(AdCampaignStatus.ENDED)]} />);
    expect(screen.getByText("Đã kết thúc")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tạm dừng" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Tiếp tục" })).toBeNull();
  });

  it("budget-exhausted row shows the label and a disabled resume button", () => {
    render(
      <AdCampaignTable
        campaigns={[
          row(AdCampaignStatus.PAUSED_BUDGET, { spent: 49600, bid: 500 }),
        ]}
      />,
    );
    expect(screen.getByText("Hết ngân sách")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tiếp tục" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
    expect(resumeAdCampaignAction).not.toHaveBeenCalled();
  });

  it("double click pauses once", async () => {
    let resolve: (v: unknown) => void = () => {};
    vi.mocked(pauseAdCampaignAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }) as never,
    );
    render(<AdCampaignTable campaigns={[row(AdCampaignStatus.ACTIVE)]} />);
    const btn = screen.getByRole("button", { name: "Tạm dừng" });
    fireEvent.click(btn);
    fireEvent.click(btn);
    await waitFor(() => expect(btn).toBeDisabled());
    expect(pauseAdCampaignAction).toHaveBeenCalledTimes(1);
    expect(pauseAdCampaignAction).toHaveBeenCalledWith("c1");
    resolve({
      ok: true,
      data: { campaign: row(AdCampaignStatus.PAUSED) },
    });
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
  });

  it("pause success refreshes the row to paused with a resume button", async () => {
    vi.mocked(pauseAdCampaignAction).mockResolvedValue({
      ok: true,
      data: { campaign: row(AdCampaignStatus.PAUSED) },
    } as never);
    render(<AdCampaignTable campaigns={[row(AdCampaignStatus.ACTIVE)]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tạm dừng" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Tiếp tục" })).toBeEnabled(),
    );
    expect(screen.queryByRole("button", { name: "Tạm dừng" })).toBeNull();
  });

  it("resume success refreshes the row to active with a pause button", async () => {
    vi.mocked(resumeAdCampaignAction).mockResolvedValue({
      ok: true,
      data: { campaign: row(AdCampaignStatus.ACTIVE) },
    } as never);
    render(<AdCampaignTable campaigns={[row(AdCampaignStatus.PAUSED)]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Tạm dừng" })).toBeEnabled(),
    );
  });

  it("failed action toasts the error and leaves the status unchanged", async () => {
    vi.mocked(resumeAdCampaignAction).mockResolvedValue({
      ok: false,
      error: "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy.",
    });
    render(<AdCampaignTable campaigns={[row(AdCampaignStatus.PAUSED)]} />);
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith(
        "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy.",
      ),
    );
    expect(screen.getByRole("button", { name: "Tiếp tục" })).toBeEnabled();
  });
});
