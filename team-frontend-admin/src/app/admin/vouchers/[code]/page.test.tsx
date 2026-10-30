import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, getToken } from "@/lib/gateway/session";
import { getVoucher } from "@/lib/gateway/vouchers";

import VoucherDetailPage from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/vouchers", () => ({ getVoucher: vi.fn() }));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

const voucher = {
  id: "v-1",
  code: "SALE 11",
  scope: "platform" as const,
  sellerId: "",
  discountType: "percent" as const,
  discountValue: 15,
  minSpend: 200000,
  maxDiscount: 50000,
  quota: 10,
  used: 10,
  startsAt: "2020-01-01T00:00:00.000Z",
  endsAt: "2099-01-01T00:00:00.000Z",
};

const page = (code: string) =>
  VoucherDetailPage({ params: Promise.resolve({ code }) });

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(getVoucher).mockResolvedValue({ status: "ok", voucher });
});

describe("VoucherDetailPage", () => {
  it("decodes the code and shows the stored voucher", async () => {
    render(await page("SALE%2011"));
    expect(getVoucher).toHaveBeenCalledWith("SALE 11");
    const details = screen.getByTestId("voucher-details");
    expect(details).toHaveTextContent("Toàn sàn");
    expect(details).toHaveTextContent("Giảm 15%, tối đa ₫50.000");
    expect(details).toHaveTextContent("10 / 10");
    expect(screen.getByTestId("voucher-state")).toHaveTextContent("Hết lượt");
  });

  it("shows not found for an unknown code", async () => {
    vi.mocked(getVoucher).mockResolvedValue({ status: "not_found" });
    render(await page("NOPE"));
    expect(screen.getByText("Không tìm thấy voucher")).toBeInTheDocument();
  });

  it("does not call the gateway for an over-long code", async () => {
    render(await page("A".repeat(65)));
    expect(getVoucher).not.toHaveBeenCalled();
    expect(screen.getByText("Không tìm thấy voucher")).toBeInTheDocument();
  });

  it("shows an alert when the promotion service is unavailable", async () => {
    vi.mocked(getVoucher).mockResolvedValue({ status: "unavailable" });
    render(await page("SALE11"));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Không tải được voucher",
    );
  });

  it("redirects a guest to login with the detail path", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await page("SALE11")).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/vouchers/SALE11")}`,
    );
  });
});
