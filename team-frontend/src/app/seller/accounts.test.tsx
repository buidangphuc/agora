import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getStorefront, listBundlesBySeller } from "@/lib/gateway/listings";
import { getWalletBalance, listLedgerEntries } from "@/lib/gateway/payment";
import { getEntitlements, listPlans } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerPlansPage from "./plans/page";
import SellerShopPage from "./shop/page";
import SellerWalletPage from "./wallet/page";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller/wallet",
  useRouter: () => ({ replace: vi.fn() }),
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/payment", () => ({
  getWalletBalance: vi.fn(),
  listLedgerEntries: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", () => ({
  listPlans: vi.fn(),
  getEntitlements: vi.fn(),
}));
vi.mock("@/lib/gateway/listings", () => ({
  getStorefront: vi.fn(),
  listBundlesBySeller: vi.fn(),
}));
vi.mock("@/features/seller/actions", () => ({
  requestWalletPayoutAction: vi.fn(),
  subscribeAction: vi.fn(),
  upsertStorefrontAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "me",
    name: "Me",
    scopes: ["listing.write"],
  });
  vi.mocked(hasScope).mockReturnValue(true);
});

function entries(n: number) {
  return Array.from({ length: n }, (_, i) => ({
    id: `e${i + 1}`,
    type: `payout-${i + 1}`,
    amount: 1000,
    status: "done",
    createdAt: "01/10/2026",
  }));
}

describe("/seller/wallet", () => {
  it("shows the balance and enables payout when positive", async () => {
    vi.mocked(getWalletBalance).mockResolvedValue(750000);
    vi.mocked(listLedgerEntries).mockResolvedValue(entries(2) as never);
    render(await SellerWalletPage({ searchParams: {} }));
    expect(screen.getByText("Số dư khả dụng")).toBeInTheDocument();
    expect(screen.getByText("₫750.000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Rút tiền" })).toBeEnabled();
  });

  it("disables payout at zero balance", async () => {
    vi.mocked(getWalletBalance).mockResolvedValue(0);
    vi.mocked(listLedgerEntries).mockResolvedValue([]);
    render(await SellerWalletPage({ searchParams: {} }));
    expect(screen.getByRole("button", { name: "Rút tiền" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByText("Chưa có giao dịch nào")).toBeInTheDocument();
  });

  it("slices the ledger by ?page= and links neighbours", async () => {
    vi.mocked(getWalletBalance).mockResolvedValue(1);
    vi.mocked(listLedgerEntries).mockResolvedValue(entries(30) as never);
    render(await SellerWalletPage({ searchParams: { page: "2" } }));
    const table = screen.getByRole("table", { name: "Lịch sử giao dịch ví" });
    expect(within(table).getAllByRole("row")).toHaveLength(11); // header + 10
    expect(within(table).getByText("payout-21")).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(
      within(nav).getByRole("link", { name: "Trang trước" }),
    ).toHaveAttribute("href", "/seller/wallet");
  });

  it("an unreadable balance is an Alert with retry, not a fake zero", async () => {
    vi.mocked(getWalletBalance).mockRejectedValue(new Error("down"));
    vi.mocked(listLedgerEntries).mockResolvedValue([]);
    render(await SellerWalletPage({ searchParams: {} }));
    expect(screen.getByText("Không tải được số dư ví.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rút tiền" })).toBeNull();
  });
});

describe("/seller/plans", () => {
  it("marks the current plan with a Tag and a disabled button", async () => {
    vi.mocked(listPlans).mockResolvedValue([
      {
        id: "p1",
        tier: 1,
        tierText: "Miễn phí",
        price: 0,
        features: ["Cơ bản"],
      },
      {
        id: "p2",
        tier: 2,
        tierText: "Pro",
        price: 199000,
        features: ["Nhiều hơn"],
      },
    ] as never);
    vi.mocked(getEntitlements).mockResolvedValue({
      tier: 1,
      tierText: "Miễn phí",
      limits: {},
    } as never);
    render(await SellerPlansPage());
    expect(screen.getAllByText("Gói hiện tại").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Gói hiện tại" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Đăng ký" })).toBeEnabled();
  });
});

describe("/seller/shop", () => {
  it("pre-fills the current display name", async () => {
    vi.mocked(getStorefront).mockResolvedValue({
      displayName: "Cửa hàng Hoa Mai",
    } as never);
    render(await SellerShopPage());
    expect(screen.getByLabelText(/Tên gian hàng/)).toHaveValue(
      "Cửa hàng Hoa Mai",
    );
    expect(
      screen.getByRole("link", { name: "Xem gian hàng công khai" }),
    ).toHaveAttribute("href", "/shop/me");
  });

  it("starts empty when the seller has no storefront", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    render(await SellerShopPage());
    expect(screen.getByLabelText(/Tên gian hàng/)).toHaveValue("");
    expect(listBundlesBySeller).not.toHaveBeenCalled();
  });
});
