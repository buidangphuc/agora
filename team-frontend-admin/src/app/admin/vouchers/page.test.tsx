import { render, screen } from "@testing-library/react";
import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getPrincipal, getToken } from "@/lib/gateway/session";
import { listPlatformVouchers } from "@/lib/gateway/vouchers";

import VouchersPage, { metadata } from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn(), notFound: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(),
}));
vi.mock("@/lib/gateway/vouchers", () => ({ listPlatformVouchers: vi.fn() }));

const admin = { id: "u-admin", name: "Admin", scopes: ["admin"] };

const voucher = {
  id: "v-1",
  code: "SALE11",
  scope: "platform" as const,
  sellerId: "",
  discountType: "fixed" as const,
  discountValue: 20000,
  minSpend: 0,
  maxDiscount: 0,
  quota: 0,
  used: 4,
  startsAt: "2020-01-01T00:00:00.000Z",
  endsAt: "2099-01-01T00:00:00.000Z",
};

const page = (sp: Record<string, string> = {}) =>
  VouchersPage({ searchParams: Promise.resolve(sp) });

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(admin);
  vi.mocked(getToken).mockReturnValue("tok");
  vi.mocked(listPlatformVouchers).mockResolvedValue({
    status: "ok",
    vouchers: [voucher],
    nextCursor: "c2",
  });
});

describe("VouchersPage", () => {
  it("lists platform vouchers with their real state and links", async () => {
    render(await page({ cursor: "c1" }));
    expect(listPlatformVouchers).toHaveBeenCalledWith("c1");
    const rows = screen.getAllByTestId("voucher-row");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Đang chạy");
    expect(rows[0]).toHaveTextContent("4 / không giới hạn");
    expect(screen.getByRole("link", { name: "SALE11" })).toHaveAttribute(
      "href",
      "/admin/vouchers/SALE11",
    );
    expect(
      screen.getByRole("link", { name: "Tạo voucher sàn" }),
    ).toHaveAttribute("href", "/admin/vouchers/new");
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      "/admin/vouchers?cursor=c2",
    );
    expect(metadata.referrer).toBe("no-referrer");
  });

  it("shows the empty state", async () => {
    vi.mocked(listPlatformVouchers).mockResolvedValue({
      status: "ok",
      vouchers: [],
      nextCursor: "",
    });
    render(await page());
    expect(screen.getByText("Chưa có voucher sàn nào")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Trang sau" })).toBeNull();
  });

  it("shows an alert when the promotion service is unavailable", async () => {
    vi.mocked(listPlatformVouchers).mockResolvedValue({
      status: "unavailable",
    });
    render(await page());
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Không tải được danh sách voucher",
    );
  });

  it("redirects a guest to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    vi.mocked(getToken).mockReturnValue(undefined);
    expect(await page()).toBeNull();
    expect(redirect).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/admin/vouchers")}`,
    );
    expect(listPlatformVouchers).not.toHaveBeenCalled();
  });

  it("shows 403 without the admin scope or when the gateway denies", async () => {
    vi.mocked(getPrincipal).mockReturnValue({ ...admin, scopes: [] });
    render(await page());
    expect(screen.getByText("Cần tài khoản Quản trị")).toBeInTheDocument();
    expect(listPlatformVouchers).not.toHaveBeenCalled();

    vi.mocked(getPrincipal).mockReturnValue(admin);
    vi.mocked(listPlatformVouchers).mockResolvedValue({ status: "forbidden" });
    render(await page());
    expect(
      screen.getByText("Gateway từ chối quyền admin cho phiên đăng nhập này."),
    ).toBeInTheDocument();
  });
});
