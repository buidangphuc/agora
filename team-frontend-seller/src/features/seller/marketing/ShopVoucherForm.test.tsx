import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { createShopVoucherAction } from "../actions";
import { ShopVoucherForm } from "./ShopVoucherForm";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }),
}));
vi.mock("../actions", () => ({
  createShopVoucherAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(createShopVoucherAction).mockResolvedValue({ ok: true });
});

function fill(over: Record<string, string> = {}) {
  const v = {
    code: "GIAM10",
    value: "10",
    quota: "100",
    start: "2030-01-01T10:00",
    end: "2030-02-01T10:00",
    ...over,
  };
  fireEvent.change(screen.getByLabelText(/Mã voucher/), {
    target: { value: v.code },
  });
  fireEvent.change(screen.getByLabelText(/Giá trị giảm/), {
    target: { value: v.value },
  });
  fireEvent.change(screen.getByLabelText(/Số lượt dùng/), {
    target: { value: v.quota },
  });
  fireEvent.change(screen.getByLabelText(/Bắt đầu/), {
    target: { value: v.start },
  });
  fireEvent.change(screen.getByLabelText(/Kết thúc/), {
    target: { value: v.end },
  });
}

const submit = () =>
  fireEvent.click(screen.getByRole("button", { name: "Tạo voucher" }));

describe("ShopVoucherForm", () => {
  it("Valid voucher is created and listed: submits once with scope-free input and no seller id", async () => {
    render(<ShopVoucherForm />);
    fill();
    submit();
    await vi.waitFor(() =>
      expect(createShopVoucherAction).toHaveBeenCalledTimes(1),
    );
    const arg = vi.mocked(createShopVoucherAction).mock.calls[0][0];
    expect(arg).toMatchObject({
      code: "GIAM10",
      discountType: DiscountType.PERCENT,
      discountValue: 10,
      quota: 100,
    });
    expect(arg.startsAt).toBeInstanceOf(Date);
    expect(arg.endsAt).toBeInstanceOf(Date);
    expect(arg.endsAt.getTime()).toBeGreaterThan(arg.startsAt.getTime());
    expect("sellerId" in arg).toBe(false);
  });

  it("rejects an empty code before any call", async () => {
    render(<ShopVoucherForm />);
    fill({ code: "" });
    submit();
    expect(
      await screen.findByText("Vui lòng nhập mã voucher"),
    ).toBeInTheDocument();
    expect(createShopVoucherAction).not.toHaveBeenCalled();
  });

  it("rejects an end before the start before any call", async () => {
    render(<ShopVoucherForm />);
    fill({ start: "2030-02-01T10:00", end: "2030-01-01T10:00" });
    submit();
    expect((await screen.findAllByRole("alert")).length).toBeGreaterThan(0);
    expect(createShopVoucherAction).not.toHaveBeenCalled();
  });

  it("Duplicate code shows the server error: message shown and form keeps its values", async () => {
    vi.mocked(createShopVoucherAction).mockResolvedValue({
      ok: false,
      error: "Mã voucher đã tồn tại",
    });
    render(<ShopVoucherForm />);
    fill();
    submit();
    expect(
      await screen.findByText("Mã voucher đã tồn tại"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/Mã voucher/)).toHaveValue("GIAM10");
    expect(screen.getByLabelText(/Số lượt dùng/)).toHaveValue(100);
  });
});
