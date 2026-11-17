import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  PlanTier,
  getEntitlements,
  getMySubscription,
  listPlans,
} from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";
import SellerPlansPage from "./page";

vi.mock("next/navigation", () => ({
  usePathname: () => "/seller/plans",
  redirect: vi.fn(),
  notFound: vi.fn(),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: vi.fn(() => {
    throw new Error("guest-redirect");
  }),
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));
vi.mock("@/lib/gateway/promotion", async () => {
  const actual = await vi
    .importActual<object>("@/lib/gateway/promotion")
    .catch(() => ({}));
  return {
    ...actual,
    PlanTier: { UNSPECIFIED: 0, FREE: 1, PRO: 2, PREMIUM: 3 },
    listPlans: vi.fn(),
    getEntitlements: vi.fn(),
    getMySubscription: vi.fn(),
  };
});
vi.mock("@/features/seller/actions", () => ({
  subscribeAction: vi.fn(),
  cancelSubscriptionAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const PLANS = [
  {
    id: "p-free",
    tier: 1,
    tierText: "Miễn phí",
    price: 0,
    features: [],
    termDays: 30,
  },
  {
    id: "p-pro",
    tier: 2,
    tierText: "Pro",
    price: 199000,
    features: [],
    termDays: 30,
  },
  {
    id: "p-prem",
    tier: 3,
    tierText: "Premium",
    price: 499000,
    features: [],
    termDays: 30,
  },
];
const PERIOD_END = "2026-12-13T10:00:00+07:00";

function sub(tier: number, autoRenew: boolean) {
  return {
    tier,
    tierText: "",
    currentPeriodEnd: tier === 1 ? "" : PERIOD_END,
    autoRenew,
  };
}

function cardOf(name: string): HTMLElement {
  let el: HTMLElement | null = screen.getByRole("heading", { name });
  while (el && !el.querySelector("button")) el = el.parentElement;
  if (!el) throw new Error(`no card for ${name}`);
  return el;
}

function tagOf(card: HTMLElement) {
  return within(card)
    .getAllByText("Gói hiện tại")
    .find((e) => e.tagName !== "BUTTON" && !e.closest("button"));
}

async function renderPage() {
  render(await SellerPlansPage());
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "me",
    name: "Me",
    scopes: ["listing.write"],
  });
  vi.mocked(hasScope).mockReturnValue(true);
  vi.mocked(listPlans).mockResolvedValue(PLANS as never);
  vi.mocked(getMySubscription).mockResolvedValue(sub(1, false));
  vi.mocked(getEntitlements).mockResolvedValue({
    tier: 1 as PlanTier,
    tierText: "Miễn phí",
    limits: { max_listings: "10" },
  });
});

describe("SellerPlansPage", () => {
  it("renders prices per term in whole VND", async () => {
    await renderPage();
    const pro = cardOf("Pro").textContent ?? "";
    const prem = cardOf("Premium").textContent ?? "";
    expect(pro).toMatch(/₫199\.000\s*\/\s*30 ngày/);
    expect(prem).toMatch(/₫499\.000\s*\/\s*30 ngày/);
    expect(document.body.textContent).not.toMatch(
      /₫(?:[5-9]\d\d|\d{4,})\.\d{3}/,
    );
  });

  it("shows period end and renewal for the current paid plan", async () => {
    vi.mocked(getMySubscription).mockResolvedValue(sub(2, true));
    vi.mocked(getEntitlements).mockResolvedValue({
      tier: 2 as PlanTier,
      tierText: "Pro",
      limits: { max_listings: "100" },
    });
    await renderPage();
    const pro = cardOf("Pro");
    expect(tagOf(pro)).toBeTruthy();
    expect(
      within(pro).getByRole("button", { name: "Gói hiện tại" }),
    ).toHaveAttribute("aria-disabled", "true");
    expect(
      within(pro).getByText("Hết hạn ngày 13/12/2026"),
    ).toBeInTheDocument();
    expect(within(pro).getByText("Tự động gia hạn: Bật")).toBeInTheDocument();
    expect(
      within(pro).getByRole("button", { name: "Hủy gia hạn" }),
    ).toBeInTheDocument();
    expect(within(cardOf("Premium")).queryByText(/Hết hạn ngày/)).toBeNull();
  });

  it("treats a missing subscription as FREE", async () => {
    await renderPage();
    const free = cardOf("Miễn phí");
    expect(tagOf(free)).toBeTruthy();
    expect(free.textContent).toContain("10");
    expect(screen.queryByText(/Hết hạn ngày/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Hủy gia hạn" })).toBeNull();
  });

  it("shows no cancel button for a paid plan with auto renew off", async () => {
    vi.mocked(getMySubscription).mockResolvedValue(sub(2, false));
    await renderPage();
    expect(screen.queryByRole("button", { name: "Hủy gia hạn" })).toBeNull();
    expect(
      within(cardOf("Pro")).getByText("Tự động gia hạn: Tắt"),
    ).toBeInTheDocument();
    expect(
      within(cardOf("Pro")).getByText("Sẽ chuyển về Miễn phí ngày 13/12/2026"),
    ).toBeInTheDocument();
  });

  it("fails closed when the subscription cannot be read", async () => {
    vi.mocked(getMySubscription).mockRejectedValue(new Error("unavailable"));
    await renderPage();
    expect(
      screen.getByText("Không tải được gói hiện tại. Thử lại sau."),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Pro" })).toBeInTheDocument();
    expect(screen.queryAllByText("Gói hiện tại")).toHaveLength(0);
    const buttons = screen.getAllByRole("button");
    expect(buttons.length).toBeGreaterThanOrEqual(3);
    for (const b of buttons) expect(b).toHaveAttribute("aria-disabled", "true");
  });

  it("renders nothing without the listing scope and issues no reads", async () => {
    vi.mocked(hasScope).mockReturnValue(false);
    expect(await SellerPlansPage()).toBeNull();
    expect(listPlans).not.toHaveBeenCalled();
    expect(getMySubscription).not.toHaveBeenCalled();
    expect(getEntitlements).not.toHaveBeenCalled();
  });
});
