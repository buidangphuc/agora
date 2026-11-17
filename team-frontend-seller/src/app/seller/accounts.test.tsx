import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";
import { getStorefront, listBundlesBySeller } from "@/lib/gateway/listings";
import { getWalletBalance, listLedgerEntries } from "@/lib/gateway/payment";
import {
  getEntitlements,
  getMySubscription,
  listPlans,
} from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import { getVerificationStatus } from "@/lib/gateway/verification";
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
vi.mock("@/lib/gateway/verification", () => ({
  getVerificationStatus: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", () => ({
  listPlans: vi.fn(),
  getEntitlements: vi.fn(),
  getMySubscription: vi.fn(),
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

function status(s: VerificationStatus) {
  vi.mocked(getVerificationStatus).mockResolvedValue({
    status: s,
    badge: s === VerificationStatus.VERIFIED,
    rejectReasonCode: "",
  });
}

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
    render(await SellerWalletPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Số dư khả dụng")).toBeInTheDocument();
    expect(screen.getByText("₫750.000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Rút tiền" })).toBeEnabled();
  });

  it("disables payout at zero balance", async () => {
    vi.mocked(getWalletBalance).mockResolvedValue(0);
    vi.mocked(listLedgerEntries).mockResolvedValue([]);
    render(await SellerWalletPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByRole("button", { name: "Rút tiền" })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByText("Chưa có giao dịch nào")).toBeInTheDocument();
  });

  it("slices the ledger by ?page= and links neighbours", async () => {
    vi.mocked(getWalletBalance).mockResolvedValue(1);
    vi.mocked(listLedgerEntries).mockResolvedValue(entries(30) as never);
    render(
      await SellerWalletPage({ searchParams: Promise.resolve({ page: "2" }) }),
    );
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
    render(await SellerWalletPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("Không tải được số dư ví.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Rút tiền" })).toBeNull();
  });
});

describe("/seller/wallet verification notice (seller-kyc-ui)", () => {
  const NOTICE = "Cần xác minh danh tính để rút tiền";

  async function open() {
    vi.mocked(getWalletBalance).mockResolvedValue(750000);
    vi.mocked(listLedgerEntries).mockResolvedValue([]);
    render(await SellerWalletPage({ searchParams: Promise.resolve({}) }));
  }

  it.each([
    ["NOT_SUBMITTED", VerificationStatus.NOT_SUBMITTED],
    ["PENDING", VerificationStatus.PENDING],
    ["REJECTED", VerificationStatus.REJECTED],
  ])("Wallet shows a notice while not verified: %s", async (_n, s) => {
    status(s);
    await open();
    expect(screen.getByText(NOTICE)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /xác minh/i })).toHaveAttribute(
      "href",
      "/seller/verification",
    );
    expect(getVerificationStatus).toHaveBeenCalledWith("me");
  });

  it("A verified seller sees no notice and can pay out", async () => {
    status(VerificationStatus.VERIFIED);
    await open();
    expect(screen.queryByText(NOTICE)).toBeNull();
    expect(screen.getByRole("button", { name: "Rút tiền" })).toBeEnabled();
  });

  it("Wallet shows a notice while not verified: a status read failure hides the notice and keeps the wallet", async () => {
    vi.mocked(getVerificationStatus).mockRejectedValue(new Error("down"));
    await open();
    expect(screen.queryByText(NOTICE)).toBeNull();
    expect(screen.getByText("₫750.000")).toBeInTheDocument();
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
    vi.mocked(getMySubscription).mockResolvedValue({
      tier: 1,
      tierText: "Miễn phí",
      currentPeriodEnd: "",
      autoRenew: false,
    } as never);
    render(await SellerPlansPage());
    expect(screen.getAllByText("Gói hiện tại").length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "Gói hiện tại" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Nâng cấp" })).toBeEnabled();
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
    ).toHaveAttribute("href", "http://localhost:3000/shop/me");
  });

  it("starts empty when the seller has no storefront", async () => {
    vi.mocked(getStorefront).mockResolvedValue(null);
    render(await SellerShopPage());
    expect(screen.getByLabelText(/Tên gian hàng/)).toHaveValue("");
    expect(listBundlesBySeller).not.toHaveBeenCalled();
  });
});
