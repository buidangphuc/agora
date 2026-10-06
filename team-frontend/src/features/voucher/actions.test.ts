import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

const createVoucher = vi.hoisted(() => vi.fn());
vi.mock("@/lib/gateway/promotion", () => ({
  createVoucher,
  previewVoucher: vi.fn(),
}));

import { createVoucherAction } from "./actions";

const input = {
  code: "SAVE10",
  scope: 1,
  discountType: 1,
  discountValue: 10,
  minSpend: 0,
  maxDiscount: 0,
  quota: 100,
};

beforeEach(() => vi.clearAllMocks());

describe("createVoucherAction", () => {
  it("returns { ok, data } with the legacy message and revalidates /vouchers", async () => {
    const voucher = { id: "v1", code: "SAVE10" };
    createVoucher.mockResolvedValue(voucher);
    const res = await createVoucherAction(input);
    expect(res).toEqual({
      ok: true,
      data: voucher,
      message: "Tạo voucher thành công!",
    });
    expect(revalidatePath).toHaveBeenCalledWith("/vouchers");
  });

  it("returns { ok: false, error } on failure and does not revalidate", async () => {
    createVoucher.mockRejectedValue(new Error("permission denied"));
    const res = await createVoucherAction(input);
    expect(res).toEqual({
      ok: false,
      error: "permission denied",
      message: "permission denied",
    });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});
