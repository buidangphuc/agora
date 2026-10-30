import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CreateCampaignForm } from "./CreateCampaignForm";
import { createCampaignAction } from "./campaignActions";
import { CAMPAIGN_ERRORS } from "./campaignForm";

vi.mock("./campaignActions", () => ({ createCampaignAction: vi.fn() }));

const stored = {
  id: "fs-42",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 3,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
};

function fill() {
  const set = (label: RegExp, value: string) =>
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  set(/^Mã tin đăng/, "l-1");
  set(/^Giá flash sale/, "99000");
  set(/^Số lượng tối đa/, "50");
  set(/^Bắt đầu/, "2099-11-01T08:00");
  set(/^Kết thúc/, "2099-11-01T12:00");
}

function openReview() {
  render(<CreateCampaignForm />);
  fill();
  fireEvent.click(
    screen.getByRole("button", { name: "Xem lại trước khi tạo" }),
  );
}

async function confirmCreate(settled: () => HTMLElement) {
  fireEvent.click(screen.getByRole("button", { name: "Xác nhận tạo" }));
  await waitFor(() => expect(settled()).toBeInTheDocument());
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createCampaignAction).mockResolvedValue({
    ok: true,
    campaign: stored,
  });
});

describe("CreateCampaignForm", () => {
  it("shows field errors and opens no dialog for an empty form", () => {
    render(<CreateCampaignForm />);
    fireEvent.click(
      screen.getByRole("button", { name: "Xem lại trước khi tạo" }),
    );
    expect(screen.getByText("Nhập mã tin đăng.")).toBeInTheDocument();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(createCampaignAction).not.toHaveBeenCalled();
  });

  it("asks for confirmation with a summary before creating", () => {
    openReview();
    const dialog = screen.getByRole("dialog", {
      name: "Xác nhận tạo flash sale",
    });
    const summary = within(dialog).getByTestId("campaign-confirm-summary");
    expect(summary).toHaveTextContent("l-1");
    expect(summary).toHaveTextContent("Tất cả phân loại");
    expect(summary).toHaveTextContent("08:00");
    expect(createCampaignAction).not.toHaveBeenCalled();
  });

  it("goes back to editing without creating", () => {
    openReview();
    fireEvent.click(screen.getByRole("button", { name: "Quay lại sửa" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(createCampaignAction).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/^Mã tin đăng/)).toHaveValue("l-1");
  });

  it("creates on confirm and shows the campaign the server returned", async () => {
    openReview();
    await confirmCreate(() => screen.getByTestId("campaign-created"));
    expect(createCampaignAction).toHaveBeenCalledWith({
      listingId: "l-1",
      variantId: "",
      salePrice: "99000",
      stockCap: "50",
      startsAt: "2099-11-01T08:00",
      endsAt: "2099-11-01T12:00",
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    const created = screen.getByTestId("campaign-created");
    expect(created).toHaveTextContent("fs-42");
    expect(created).toHaveTextContent("3 / 50");
    expect(
      screen.getByRole("link", { name: "Xem chiến dịch đang chạy" }),
    ).toHaveAttribute("href", "/admin/campaigns");
  });

  it("keeps the dialog open with the error when the audit trail is down", async () => {
    vi.mocked(createCampaignAction).mockResolvedValue({
      ok: false,
      error: CAMPAIGN_ERRORS.unavailable,
    });
    openReview();
    await confirmCreate(() => screen.getByTestId("campaign-confirm-error"));
    const dialog = screen.getByRole("dialog");
    expect(
      within(dialog).getByTestId("campaign-confirm-error"),
    ).toHaveTextContent(CAMPAIGN_ERRORS.unavailable);
    expect(screen.queryByTestId("campaign-created")).toBeNull();
  });

  it("closes the dialog and warns when the outcome is unknown", async () => {
    vi.mocked(createCampaignAction).mockResolvedValue({
      ok: false,
      error: CAMPAIGN_ERRORS.timeout,
      uncertain: true,
    });
    openReview();
    await confirmCreate(() => screen.getByTestId("campaign-form-warning"));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByTestId("campaign-form-warning")).toHaveTextContent(
      CAMPAIGN_ERRORS.timeout,
    );
  });

  it("shows server field errors on the form", async () => {
    vi.mocked(createCampaignAction).mockResolvedValue({
      ok: false,
      error: CAMPAIGN_ERRORS.invalid,
      fieldErrors: { endsAt: "Thời điểm kết thúc phải ở tương lai." },
    });
    openReview();
    await confirmCreate(() =>
      screen.getByText("Thời điểm kết thúc phải ở tương lai."),
    );
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      screen.getByText("Thời điểm kết thúc phải ở tương lai."),
    ).toBeInTheDocument();
  });
});
