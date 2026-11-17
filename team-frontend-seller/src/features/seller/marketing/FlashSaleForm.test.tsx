import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createCampaignAction, searchMyListingsAction } from "../actions";
import { FlashSaleForm } from "./FlashSaleForm";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}));
vi.mock("../actions", () => ({
  createCampaignAction: vi.fn(),
  searchMyListingsAction: vi.fn(),
}));

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.mocked(createCampaignAction).mockResolvedValue({ ok: true });
  vi.mocked(searchMyListingsAction).mockResolvedValue({
    ok: true,
    data: {
      page: {
        items: [{ id: "l1", title: "Áo A" }],
        nextCursor: "",
        total: 1,
      },
    },
  } as never);
});
afterEach(() => vi.useRealTimers());

async function pick() {
  fireEvent.change(screen.getByLabelText(/Tìm sản phẩm/), {
    target: { value: "ao" },
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
  fireEvent.click(screen.getByRole("button", { name: /Áo A/ }));
}

function fill(price = "50000", cap = "10") {
  fireEvent.change(screen.getByLabelText(/Giá flash sale/), {
    target: { value: price },
  });
  fireEvent.change(screen.getByLabelText(/Số lượng tối đa/), {
    target: { value: cap },
  });
  fireEvent.change(screen.getByLabelText(/Bắt đầu/), {
    target: { value: "2030-01-01T10:00" },
  });
  fireEvent.change(screen.getByLabelText(/Kết thúc/), {
    target: { value: "2030-01-02T10:00" },
  });
}

const submit = () =>
  fireEvent.click(screen.getByRole("button", { name: "Tạo flash sale" }));

describe("FlashSaleForm", () => {
  it("Valid campaign is created and listed: one call with the picked listing, a key and no seller id", async () => {
    render(<FlashSaleForm />);
    await pick();
    fill();
    await act(async () => submit());
    expect(createCampaignAction).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(createCampaignAction).mock.calls[0][0];
    expect(arg).toMatchObject({
      listingId: "l1",
      salePrice: 50000,
      stockCap: 10,
    });
    expect(arg.idempotencyKey).toEqual(expect.any(String));
    expect(arg.idempotencyKey).not.toBe("");
    expect(arg.startsAt).toBeInstanceOf(Date);
    expect("sellerId" in arg).toBe(false);
  });

  it("double submit sends one CreateCampaign with one idempotency key", async () => {
    let release: (v: { ok: true }) => void = () => {};
    vi.mocked(createCampaignAction).mockImplementationOnce(
      () =>
        new Promise((r) => {
          release = r;
        }),
    );
    render(<FlashSaleForm />);
    await pick();
    fill();
    const btn = screen.getByRole("button", { name: "Tạo flash sale" });
    await act(async () => {
      fireEvent.click(btn);
      fireEvent.click(btn);
    });
    expect(createCampaignAction).toHaveBeenCalledTimes(1);
    await act(async () => release({ ok: true }));
  });

  it("double submit sends one CreateCampaign with one idempotency key: a retry after failure reuses the key", async () => {
    vi.mocked(createCampaignAction).mockResolvedValue({
      ok: false,
      error: "lỗi tạm thời",
    });
    render(<FlashSaleForm />);
    await pick();
    fill();
    await act(async () => submit());
    await act(async () => submit());
    expect(createCampaignAction).toHaveBeenCalledTimes(2);
    const [a, b] = vi
      .mocked(createCampaignAction)
      .mock.calls.map((c) => c[0].idempotencyKey);
    expect(a).toBe(b);
  });

  it("rejects a missing listing before any call", async () => {
    render(<FlashSaleForm />);
    fill();
    await act(async () => submit());
    expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    expect(createCampaignAction).not.toHaveBeenCalled();
  });

  it("rejects sale price 0 and cap 0 before any call", async () => {
    render(<FlashSaleForm />);
    await pick();
    fill("0", "10");
    await act(async () => submit());
    expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    expect(createCampaignAction).not.toHaveBeenCalled();

    fill("50000", "0");
    await act(async () => submit());
    expect(createCampaignAction).not.toHaveBeenCalled();
  });

  it("shows the server message and keeps the form values", async () => {
    vi.mocked(createCampaignAction).mockResolvedValue({
      ok: false,
      error: "Giá flash sale phải thấp hơn giá niêm yết",
    });
    render(<FlashSaleForm />);
    await pick();
    fill();
    await act(async () => submit());
    expect(
      screen.getByText("Giá flash sale phải thấp hơn giá niêm yết"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/Giá flash sale/)).toHaveValue(50000);
    expect(screen.getByLabelText(/Số lượng tối đa/)).toHaveValue(10);
  });
});
