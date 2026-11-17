import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { getWalletBalance, listLedgerEntries } from "@/lib/gateway/payment";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerWalletPage from "./page";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/seller/wallet",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({
  getWalletBalance: vi.fn(),
  listLedgerEntries: vi.fn(),
  requestWalletPayout: vi.fn(),
}));
vi.mock("@/features/seller/actions", () => ({
  requestPayoutAction: vi.fn(),
}));

function entry(id: string, type: string, amount: number) {
  return {
    id,
    sellerId: "seller-1",
    type,
    amount,
    status: "COMPLETED",
    createdAt: "01/11/2026",
  };
}

async function renderPage() {
  const ui = await SellerWalletPage({ searchParams: Promise.resolve({}) });
  return render(ui);
}

describe("Wallet ledger type labels", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(getPrincipal).mockReturnValue({
      id: "seller-1",
      scopes: ["listing.write"],
    } as never);
    vi.mocked(hasScope).mockReturnValue(true);
    vi.mocked(getWalletBalance).mockResolvedValue(500000 as never);
  });

  it("ledger rows show Vietnamese labels, fallback and dash", async () => {
    vi.mocked(listLedgerEntries).mockResolvedValue([
      entry("e1", "ORDER_SETTLEMENT", 310000),
      entry("e2", "COMMISSION", -14000),
      entry("e3", "FUTURE_TYPE", 1000),
      entry("e4", "", 2000),
    ] as never);
    await renderPage();
    expect(screen.getByText("Thanh toán đơn hàng")).toBeTruthy();
    expect(screen.getByText("Phí hoa hồng")).toBeTruthy();
    expect(screen.getByText("-₫14.000")).toBeTruthy();
    expect(screen.getByText("₫310.000")).toBeTruthy();
    expect(screen.getByText("FUTURE_TYPE")).toBeTruthy();
    expect(screen.queryByText("COMMISSION")).toBeNull();
    expect(screen.queryByText("ORDER_SETTLEMENT")).toBeNull();
  });

  it("ledger error keeps the retry state", async () => {
    vi.mocked(listLedgerEntries).mockRejectedValue(new Error("UNAVAILABLE"));
    await renderPage();
    expect(screen.getByText("Không tải được lịch sử giao dịch.")).toBeTruthy();
    expect(screen.getAllByText("Thử lại").length).toBeGreaterThan(0);
    expect(screen.queryByText("Phí hoa hồng")).toBeNull();
    expect(screen.queryByText("Thanh toán đơn hàng")).toBeNull();
  });

  it("guest is redirected to login", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null as never);
    await expect(
      SellerWalletPage({ searchParams: Promise.resolve({}) }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirectGuestToLogin).toHaveBeenCalled();
    expect(listLedgerEntries).not.toHaveBeenCalled();
  });
});
