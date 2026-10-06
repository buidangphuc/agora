import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DiscountType } from "@/generated/platform/promotion/v1/promotion_pb.js";
import { trackEcommerce } from "@/lib/analytics";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { setupUser } from "@/test/user";

import { VoucherModal } from "./VoucherModal";
import { previewVoucherAction } from "./actions";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
const nav = vi.hoisted(() => ({ replace: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: nav.replace }),
  usePathname: () => "/checkout",
  useSearchParams: () => new URLSearchParams("step=payment&addr=a1"),
}));
vi.mock("./actions", () => ({ previewVoucherAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

const voucher: ViewVoucher = {
  id: "v1",
  code: "SAVE10",
  scope: 0,
  scopeText: "",
  sellerId: "",
  discountType: DiscountType.PERCENT,
  discountTypeText: "Giảm %",
  discountValue: 10,
  minSpend: 100000,
  maxDiscount: 50000,
  quota: 0,
  used: 0,
  startsAt: "",
  endsAt: "",
};

function setup(over: Partial<React.ComponentProps<typeof VoucherModal>> = {}) {
  return render(
    <VoucherModal
      vouchers={[voucher]}
      subtotal={300000}
      sellerId="s1"
      appliedDiscount={0}
      {...over}
    />,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("VoucherModal", () => {
  it("applies a valid code: loading, server discount, toast, closes, sets ?voucher=", async () => {
    const user = setupUser();
    let resolve!: (v: Awaited<ReturnType<typeof previewVoucherAction>>) => void;
    const pending = new Promise<
      Awaited<ReturnType<typeof previewVoucherAction>>
    >((r) => {
      resolve = r;
    });
    vi.mocked(previewVoucherAction).mockReturnValue(pending);
    setup();
    await user.click(screen.getByRole("button", { name: "Chọn hoặc nhập mã" }));
    await user.type(
      screen.getByRole("textbox", { name: "Nhập mã giảm giá" }),
      "SAVE10",
    );
    await user.click(screen.getByRole("button", { name: "Áp dụng" }));

    expect(previewVoucherAction).toHaveBeenCalledWith("SAVE10", 300000, "s1");
    expect(screen.getByRole("button", { name: "Áp dụng" })).toHaveAttribute(
      "aria-busy",
      "true",
    );

    const { act } = await import("@testing-library/react");
    await act(async () => {
      resolve({
        valid: true,
        reason: "",
        discountAmount: 30000,
        voucherId: "v1",
      });
      await pending;
    });
    expect(toast.success).toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(nav.replace).toHaveBeenCalledWith(
      "/checkout?step=payment&addr=a1&voucher=SAVE10",
    );
    expect(trackEcommerce).toHaveBeenCalledWith("apply_promotion", {
      coupon: "SAVE10",
      value: 30000,
      properties: { valid: "true" },
    });
  });

  it("rejects an invalid code with the reason, keeps the modal open and applies nothing", async () => {
    const user = setupUser();
    vi.mocked(previewVoucherAction).mockResolvedValue({
      valid: false,
      reason: "Mã không tồn tại",
      discountAmount: 0,
      voucherId: "",
    });
    setup({ appliedCode: "OLD", appliedDiscount: 1000 });
    await user.click(screen.getByRole("button", { name: "Đổi mã" }));
    await user.type(
      screen.getByRole("textbox", { name: "Nhập mã giảm giá" }),
      "BOGUS-NOPE-999",
    );
    await user.click(screen.getByRole("button", { name: "Áp dụng" }));

    expect(screen.getByText("Mã không tồn tại")).toBeInTheDocument();
    expect(toast.error).toHaveBeenCalledWith("Mã không tồn tại");
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(nav.replace).not.toHaveBeenCalled();
    expect(trackEcommerce).toHaveBeenCalledWith("apply_promotion", {
      coupon: "BOGUS-NOPE-999",
      value: 0,
      properties: { valid: "false" },
    });
  });

  it("traps focus while open and Escape closes it returning focus to the selector", async () => {
    const user = setupUser();
    setup();
    const opener = screen.getByRole("button", { name: "Chọn hoặc nhập mã" });
    await user.click(opener);
    const dialog = screen.getByRole("dialog");
    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(opener).toHaveFocus();
  });

  it("shows an Empty block when there are no vouchers and keeps the code input usable", async () => {
    const user = setupUser();
    setup({ vouchers: [] });
    await user.click(screen.getByRole("button", { name: "Chọn hoặc nhập mã" }));
    expect(screen.getByText("Chưa có voucher khả dụng")).toBeInTheDocument();
    await user.type(
      screen.getByRole("textbox", { name: "Nhập mã giảm giá" }),
      "X",
    );
    expect(screen.getByRole("button", { name: "Áp dụng" })).toBeEnabled();
  });

  it("picking an available voucher fills the code input", async () => {
    const user = setupUser();
    setup();
    await user.click(screen.getByRole("button", { name: "Chọn hoặc nhập mã" }));
    await user.click(screen.getByRole("radio", { name: /SAVE10/ }));
    expect(
      screen.getByRole("textbox", { name: "Nhập mã giảm giá" }),
    ).toHaveValue("SAVE10");
  });

  it("removes an applied voucher through its Tag", async () => {
    const user = setupUser();
    setup({ appliedCode: "SAVE10", appliedDiscount: 30000 });
    expect(screen.getByText("SAVE10")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Bỏ mã SAVE10" }));
    expect(nav.replace).toHaveBeenCalledWith("/checkout?step=payment&addr=a1");
  });
});
