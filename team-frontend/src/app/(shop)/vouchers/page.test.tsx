import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const listVouchers = vi.hoisted(() => vi.fn());
const getPrincipal = vi.hoisted(() => vi.fn());
const hasScope = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/promotion", async () => {
  const actual = await vi.importActual<
    typeof import("@/generated/platform/promotion/v1/promotion_pb.js")
  >("@/generated/platform/promotion/v1/promotion_pb.js");
  return {
    listVouchers,
    DiscountType: actual.DiscountType,
    VoucherScope: actual.VoucherScope,
  };
});
vi.mock("@/lib/gateway/session", () => ({ getPrincipal, hasScope }));
vi.mock("@/features/voucher/VoucherManager", () => ({
  VoucherManager: () => <div>MANAGER</div>,
}));

import VouchersPage from "./page";

const fixed = {
  id: "v2",
  code: "MINUS50K",
  scope: 1,
  scopeText: "Shop",
  sellerId: "s",
  discountType: 2,
  discountTypeText: "Giảm tiền",
  discountValue: 50000,
  minSpend: 0,
  maxDiscount: 0,
  quota: 0,
  used: 0,
  startsAt: "",
  endsAt: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  listVouchers.mockResolvedValue([fixed]);
  getPrincipal.mockReturnValue(null);
  hasScope.mockReturnValue(false);
});

describe("VouchersPage", () => {
  it("renders the real vouchers filtered by ?type=", async () => {
    render(await VouchersPage({ searchParams: { type: "percent" } }));
    expect(screen.queryByTestId("voucher-card")).toBeNull();
    render(await VouchersPage({ searchParams: { type: "fixed" } }));
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
  });

  it("falls back to all for an unknown type", async () => {
    render(await VouchersPage({ searchParams: { type: "zzz" } }));
    expect(screen.getByTestId("voucher-card")).toBeInTheDocument();
  });

  it("hides the seller manager without the listing.write scope", async () => {
    getPrincipal.mockReturnValue({ sub: "u" });
    hasScope.mockReturnValue(false);
    render(await VouchersPage({ searchParams: {} }));
    expect(screen.queryByText("MANAGER")).toBeNull();
  });

  it("hides the seller manager for anonymous visitors", async () => {
    render(await VouchersPage({ searchParams: {} }));
    expect(screen.queryByText("MANAGER")).toBeNull();
  });

  it("shows the seller manager with listing.write", async () => {
    getPrincipal.mockReturnValue({ sub: "u" });
    hasScope.mockImplementation((s: string) => s === "listing.write");
    render(await VouchersPage({ searchParams: {} }));
    expect(screen.getByText("MANAGER")).toBeInTheDocument();
  });

  it("renders Empty when the gateway returned nothing", async () => {
    listVouchers.mockResolvedValue([]);
    render(await VouchersPage({ searchParams: {} }));
    expect(
      screen.getByRole("link", { name: "Xem sản phẩm" }),
    ).toBeInTheDocument();
  });
});
