import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import { createPlatformVoucher } from "@/lib/gateway/vouchers";

import { createPlatformVoucherAction } from "./actions";
import { VOUCHER_MESSAGES } from "./voucherMessages";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/vouchers", () => ({ createPlatformVoucher: vi.fn() }));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

const values = {
  code: "sale11",
  discountType: "percent",
  discountValue: "10",
  minSpend: "0",
  maxDiscount: "0",
  quota: "100",
  startsAt: "2099-01-01T00:00",
  endsAt: "2099-02-01T00:00",
};

const stored = {
  id: "v-1",
  code: "SALE11",
  scope: "platform" as const,
  sellerId: "",
  discountType: "percent" as const,
  discountValue: 10,
  minSpend: 0,
  maxDiscount: 0,
  quota: 100,
  used: 0,
  startsAt: "2098-12-31T17:00:00.000Z",
  endsAt: "2099-01-31T17:00:00.000Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
});

describe("createPlatformVoucherAction", () => {
  it("creates with the validated input and returns the stored voucher", async () => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({
      status: "ok",
      voucher: stored,
    });
    const res = await createPlatformVoucherAction(values);
    expect(res).toEqual({ ok: true, voucher: stored });
    expect(vi.mocked(createPlatformVoucher).mock.calls[0][0]).toMatchObject({
      code: "SALE11",
      discountType: "percent",
      discountValue: 10,
      quota: 100,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/vouchers");
  });

  it("returns field errors and never calls the gateway for invalid input", async () => {
    const res = await createPlatformVoucherAction({ ...values, code: "x" });
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.error).toBe(VOUCHER_MESSAGES.fields);
      expect(res.fieldErrors?.code).toBeTruthy();
    }
    expect(createPlatformVoucher).not.toHaveBeenCalled();
  });

  it("refuses a session without the admin scope", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: ["order.read"],
    });
    expect(await createPlatformVoucherAction(values)).toEqual({
      ok: false,
      error: VOUCHER_MESSAGES.forbidden,
    });
    expect(createPlatformVoucher).not.toHaveBeenCalled();
  });

  it("sends a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(createPlatformVoucherAction(values)).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/vouchers/new")}`,
    );
  });

  it("explains that nothing was created when audit is down", async () => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({
      status: "unavailable",
    });
    const res = await createPlatformVoucherAction(values);
    expect(res).toEqual({ ok: false, error: VOUCHER_MESSAGES.auditDown });
    expect(VOUCHER_MESSAGES.auditDown).toMatch(/kiểm toán/);
    expect(VOUCHER_MESSAGES.auditDown).toMatch(/chưa có voucher nào/);
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it.each([
    ["forbidden", VOUCHER_MESSAGES.forbidden],
    ["invalid", VOUCHER_MESSAGES.invalid],
    ["timeout", VOUCHER_MESSAGES.timeout],
    ["failed", VOUCHER_MESSAGES.failed],
    ["not_found", VOUCHER_MESSAGES.failed],
  ] as const)("maps %s to its Vietnamese message", async (status, message) => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({ status });
    expect(await createPlatformVoucherAction(values)).toEqual({
      ok: false,
      error: message,
    });
  });
});
