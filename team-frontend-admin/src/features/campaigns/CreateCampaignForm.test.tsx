import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CreateCampaignForm } from "./CreateCampaignForm";
import { createCampaignAction } from "./actions";

vi.mock("./actions", () => ({
  createCampaignAction: vi.fn(),
  cancelCampaignAction: vi.fn(),
  updateCampaignAction: vi.fn(),
}));

const stored = {
  id: "fs-42",
  listingId: "l-1",
  variantId: "",
  salePrice: 99000,
  stockCap: 50,
  stockSold: 0,
  startsAt: "2099-11-01T01:00:00.000Z",
  endsAt: "2099-11-01T05:00:00.000Z",
  status: "scheduled" as const,
  cancelledAt: "",
};

const KEY_RE = /^[A-Za-z0-9_-]{8,64}$/;

function fill(endsAt = "2099-11-01T12:00") {
  const set = (label: RegExp, value: string) =>
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  set(/^Mã tin đăng/, "l-1");
  set(/^Giá flash sale/, "99000");
  set(/^Số lượng tối đa/, "50");
  set(/^Bắt đầu/, "2099-11-01T08:00");
  set(/^Kết thúc/, endsAt);
}

function review() {
  fireEvent.click(
    screen.getByRole("button", { name: "Xem lại trước khi tạo" }),
  );
}

async function confirm() {
  const before = vi.mocked(createCampaignAction).mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "Xác nhận tạo" }));
  await waitFor(() =>
    expect(vi.mocked(createCampaignAction).mock.calls.length).toBe(before + 1),
  );
}

const keyOfCall = (n: number) =>
  vi.mocked(createCampaignAction).mock.calls[n]?.[1];

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createCampaignAction).mockResolvedValue({
    ok: true,
    campaign: stored,
  });
});

describe("CreateCampaignForm", () => {
  it("sends a well-formed idempotency key with the draft", async () => {
    render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    expect(createCampaignAction).toHaveBeenCalledWith(
      {
        listingId: "l-1",
        variantId: "",
        salePrice: "99000",
        stockCap: "50",
        startsAt: "2099-11-01T08:00",
        endsAt: "2099-11-01T12:00",
      },
      expect.stringMatching(KEY_RE),
    );
    await waitFor(() =>
      expect(screen.getByTestId("campaign-created")).toHaveTextContent("fs-42"),
    );
  });

  it("create retry reuses the key after a timeout", async () => {
    vi.mocked(createCampaignAction).mockResolvedValueOnce({
      ok: false,
      error: "Chưa rõ chiến dịch đã được tạo hay chưa.",
      uncertain: true,
    });
    render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    review();
    await confirm();
    expect(createCampaignAction).toHaveBeenCalledTimes(2);
    expect(keyOfCall(0)).toMatch(KEY_RE);
    expect(keyOfCall(1)).toBe(keyOfCall(0));
  });

  it("create retry reuses the key when the confirm dialog stays open on an outage", async () => {
    vi.mocked(createCampaignAction).mockResolvedValueOnce({
      ok: false,
      error: "Hệ thống kiểm toán đang tạm ngưng.",
    });
    render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    await waitFor(() =>
      expect(screen.getByTestId("campaign-confirm-error")).toBeInTheDocument(),
    );
    await confirm();
    expect(keyOfCall(1)).toBe(keyOfCall(0));
  });

  it("create retry reuses the key after the server refused the fields", async () => {
    vi.mocked(createCampaignAction).mockResolvedValueOnce({
      ok: false,
      error: "Dữ liệu không hợp lệ.",
      fieldErrors: { salePrice: "Giá flash sale phải thấp hơn giá tin đăng." },
    });
    render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    await waitFor(() =>
      expect(
        screen.getByText("Giá flash sale phải thấp hơn giá tin đăng."),
      ).toBeInTheDocument(),
    );
    fireEvent.change(screen.getByLabelText(/^Giá flash sale/), {
      target: { value: "90000" },
    });
    review();
    await confirm();
    expect(keyOfCall(1)).toBe(keyOfCall(0));
  });

  it("a fresh form session gets its own key", async () => {
    const first = render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    first.unmount();
    render(<CreateCampaignForm />);
    fill();
    review();
    await confirm();
    expect(keyOfCall(1)).toMatch(KEY_RE);
    expect(keyOfCall(1)).not.toBe(keyOfCall(0));
  });

  it("validates the window client-side: an end before the start never reaches the server", () => {
    render(<CreateCampaignForm />);
    fill("2099-11-01T07:00");
    review();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(createCampaignAction).not.toHaveBeenCalled();
  });

  it("validates price and stock client-side: zero values never reach the server", () => {
    render(<CreateCampaignForm />);
    fill();
    fireEvent.change(screen.getByLabelText(/^Giá flash sale/), {
      target: { value: "0" },
    });
    fireEvent.change(screen.getByLabelText(/^Số lượng tối đa/), {
      target: { value: "0" },
    });
    review();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(createCampaignAction).not.toHaveBeenCalled();
  });
});
