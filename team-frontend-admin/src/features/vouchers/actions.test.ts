import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal } from "@/lib/gateway/session";
import {
  createPlatformVoucher,
  deactivateVoucher,
  updateVoucher,
} from "@/lib/gateway/vouchers";

import {
  createPlatformVoucherAction,
  deactivateVoucherAction,
  updateVoucherAction,
} from "./actions";
import { VOUCHER_MESSAGES } from "./voucherMessages";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("@/lib/gateway/vouchers", () => ({
  createPlatformVoucher: vi.fn(),
  deactivateVoucher: vi.fn(),
  updateVoucher: vi.fn(),
}));

const admin = {
  id: "u-admin",
  name: "Admin",
  scopes: ["admin", "promotion.admin"],
};

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
  active: true,
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

  it("refuses a session without the admin scope (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: ["admin"],
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

  it("V26 explains that nothing was created when audit is down", async () => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({
      status: "audit_unavailable",
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
    ["unavailable", VOUCHER_MESSAGES.failed],
  ] as const)("maps %s to its Vietnamese message", async (status, message) => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({ status });
    expect(await createPlatformVoucherAction(values)).toEqual({
      ok: false,
      error: message,
    });
  });
});

describe("create outage and duplicate messages", () => {
  it("V27 explains that nothing was created when promotion is down", async () => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({
      status: "promotion_unavailable",
    });
    const res = await createPlatformVoucherAction(values);
    expect(res).toEqual({ ok: false, error: VOUCHER_MESSAGES.promotionDown });
    expect(VOUCHER_MESSAGES.promotionDown).toMatch(/khuyến mãi/);
    expect(VOUCHER_MESSAGES.promotionDown).toMatch(/chưa có voucher nào/);
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("V20 puts the duplicate message on the code field", async () => {
    vi.mocked(createPlatformVoucher).mockResolvedValue({ status: "duplicate" });
    const res = await createPlatformVoucherAction(values);
    expect(res.ok).toBe(false);
    if (!res.ok) {
      expect(res.fieldErrors?.code).toBe(VOUCHER_MESSAGES.duplicate);
      expect(Object.keys(res.fieldErrors ?? {})).toEqual(["code"]);
    }
    expect(VOUCHER_MESSAGES.duplicate).toBeTruthy();
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("V27c the audit and promotion messages differ and never mention each other", () => {
    expect(VOUCHER_MESSAGES.auditDown).not.toBe(VOUCHER_MESSAGES.promotionDown);
    expect(VOUCHER_MESSAGES.auditDown).toMatch(/kiểm toán/);
    expect(VOUCHER_MESSAGES.auditDown).not.toMatch(/khuyến mãi/);
    expect(VOUCHER_MESSAGES.promotionDown).toMatch(/khuyến mãi/);
    expect(VOUCHER_MESSAGES.promotionDown).not.toMatch(/kiểm toán/);
    expect(VOUCHER_MESSAGES.auditDownChange).not.toBe(
      VOUCHER_MESSAGES.promotionDownChange,
    );
    expect(VOUCHER_MESSAGES.auditDownChange).toMatch(/kiểm toán/);
    expect(VOUCHER_MESSAGES.auditDownChange).not.toMatch(/khuyến mãi/);
    expect(VOUCHER_MESSAGES.promotionDownChange).toMatch(/khuyến mãi/);
    expect(VOUCHER_MESSAGES.promotionDownChange).not.toMatch(/kiểm toán/);
  });

  it("V26a V27b the change messages say nothing was changed, not created", () => {
    for (const m of [
      VOUCHER_MESSAGES.auditDownChange,
      VOUCHER_MESSAGES.promotionDownChange,
    ]) {
      expect(m).toMatch(/chưa có thay đổi nào/);
      expect(m).not.toMatch(/chưa có voucher nào/);
    }
  });
});

describe("deactivateVoucherAction", () => {
  const stoppedVoucher = { ...stored, active: false };

  it("V28 stops the voucher by id and refreshes the list", async () => {
    vi.mocked(deactivateVoucher).mockResolvedValue({
      status: "ok",
      voucher: stoppedVoucher,
    });
    const res = await deactivateVoucherAction("v-1");
    expect(res).toEqual({ ok: true, voucher: stoppedVoucher });
    expect(deactivateVoucher).toHaveBeenCalledWith("v-1");
    expect(revalidatePath).toHaveBeenCalledWith("/admin/vouchers");
  });

  it.each([
    ["audit_unavailable", VOUCHER_MESSAGES.auditDownChange],
    ["promotion_unavailable", VOUCHER_MESSAGES.promotionDownChange],
    ["forbidden", VOUCHER_MESSAGES.forbidden],
    ["timeout", VOUCHER_MESSAGES.timeout],
  ] as const)("V26a V27b maps %s", async (status, message) => {
    vi.mocked(deactivateVoucher).mockResolvedValue({ status });
    const res = await deactivateVoucherAction("v-1");
    expect(res).toEqual({ ok: false, error: message });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("V32 refuses a session without the admin scope before any request", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: ["order.read"],
    });
    const res = await deactivateVoucherAction("v-1");
    expect(res.ok).toBe(false);
    expect(deactivateVoucher).not.toHaveBeenCalled();
  });

  it("V32 refuses a session without the admin scope before any request (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: ["admin"],
    });
    const res = await deactivateVoucherAction("v-1");
    expect(res.ok).toBe(false);
    expect(deactivateVoucher).not.toHaveBeenCalled();
  });

  it("V32 sends a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    await expect(deactivateVoucherAction("v-1")).rejects.toThrow(
      "NEXT_REDIRECT",
    );
    expect(deactivateVoucher).not.toHaveBeenCalled();
  });
});

describe("updateVoucherAction", () => {
  const edit = { endsAt: "2099-03-01T00:00", quota: "200" };
  const updated = {
    ...stored,
    quota: 200,
    endsAt: "2099-02-28T17:00:00.000Z",
  };

  it("V29 sends the validated end date and quota and refreshes the list", async () => {
    vi.mocked(updateVoucher).mockResolvedValue({
      status: "ok",
      voucher: updated,
    });
    const res = await updateVoucherAction(stored, edit);
    expect(res).toEqual({ ok: true, voucher: updated });
    expect(updateVoucher).toHaveBeenCalledWith("v-1", {
      endsAt: new Date("2099-02-28T17:00:00.000Z"),
      quota: 200,
    });
    expect(revalidatePath).toHaveBeenCalledWith("/admin/vouchers");
  });

  it("V30 returns field errors and sends no request for a quota below the current one", async () => {
    const res = await updateVoucherAction(stored, { ...edit, quota: "50" });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.fieldErrors?.quota).toBeTruthy();
    expect(updateVoucher).not.toHaveBeenCalled();
  });

  it("V30 refuses quota 0 for a limited voucher", async () => {
    const res = await updateVoucherAction(stored, { ...edit, quota: "0" });
    expect(res.ok).toBe(false);
    expect(updateVoucher).not.toHaveBeenCalled();
  });

  it("V30 returns a field error for an end date not after the start", async () => {
    const res = await updateVoucherAction(stored, {
      ...edit,
      endsAt: "2099-01-01T00:00",
    });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.fieldErrors?.endsAt).toBeTruthy();
    expect(updateVoucher).not.toHaveBeenCalled();
  });

  it.each([
    ["audit_unavailable", VOUCHER_MESSAGES.auditDownChange],
    ["promotion_unavailable", VOUCHER_MESSAGES.promotionDownChange],
    ["forbidden", VOUCHER_MESSAGES.forbidden],
    ["timeout", VOUCHER_MESSAGES.timeout],
  ] as const)("V26a V27b maps %s", async (status, message) => {
    vi.mocked(updateVoucher).mockResolvedValue({ status });
    const res = await updateVoucherAction(stored, edit);
    expect(res).toEqual({ ok: false, error: message });
    expect(revalidatePath).not.toHaveBeenCalled();
  });

  it("V32 refuses a session without the admin scope before any request", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: [],
    });
    const res = await updateVoucherAction(stored, edit);
    expect(res.ok).toBe(false);
    expect(updateVoucher).not.toHaveBeenCalled();
  });

  it("V32 refuses a session without the admin scope before any request (admin token without promotion.admin)", async () => {
    vi.mocked(getPrincipal).mockReturnValue({
      id: "u-1",
      name: "Buyer",
      scopes: ["admin"],
    });
    const res = await updateVoucherAction(stored, edit);
    expect(res.ok).toBe(false);
    expect(updateVoucher).not.toHaveBeenCalled();
  });
});
