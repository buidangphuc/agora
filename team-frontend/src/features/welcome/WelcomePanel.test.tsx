import { Timestamp } from "@bufbuild/protobuf";
import { render, screen, within } from "@testing-library/react";
import { cookies } from "next/headers";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import {
  DiscountType,
  VoucherKind,
  VoucherScope,
} from "@/generated/platform/promotion/v1/promotion_pb.js";
import { listAddresses } from "@/lib/gateway/addresses";
import { makeClients } from "@/lib/gateway/client";
import { listFollowedSellers } from "@/lib/gateway/engagement";
import { listBuyerOrders } from "@/lib/gateway/orders";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { listMyVouchers } from "@/lib/gateway/promotion";
import { getPrincipal } from "@/lib/gateway/session";
import { setupUser } from "@/test/user";

import { WelcomePanel } from "./WelcomePanel";
import { dismissWelcome } from "./actions";

vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  getToken: vi.fn(() => "test-token"),
}));
vi.mock("@/lib/gateway/client", () => ({ makeClients: vi.fn() }));
vi.mock("@/lib/gateway/promotion", async () => {
  const actual = await vi.importActual<
    typeof import("@/generated/platform/promotion/v1/promotion_pb.js")
  >("@/generated/platform/promotion/v1/promotion_pb.js");
  return {
    listMyVouchers: vi.fn(),
    DiscountType: actual.DiscountType,
    VoucherScope: actual.VoucherScope,
    VoucherKind: actual.VoucherKind,
  };
});
vi.mock("@/lib/gateway/engagement", () => ({ listFollowedSellers: vi.fn() }));
vi.mock("@/lib/gateway/addresses", () => ({ listAddresses: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ listBuyerOrders: vi.fn() }));
vi.mock("./actions", () => ({ dismissWelcome: vi.fn() }));

const jar = () =>
  cookies() as unknown as {
    set: (name: string, value: string) => void;
    delete: (name: string) => void;
  };

const NOW = new Date("2026-11-06T12:00:00Z");
const MIN = 60 * 1000;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;

const BANNER_COOKIE = "agora_welcome_dismissed";
const CHECKLIST_COOKIE = "agora_checklist_dismissed";
const PENDING = "Voucher đang được chuẩn bị";

function voucher(over: Partial<ViewVoucher> = {}): ViewVoucher {
  return {
    id: "v1",
    code: "WELCOME50",
    scope: VoucherScope.PLATFORM,
    scopeText: "Toàn sàn",
    sellerId: "",
    discountType: DiscountType.FIXED,
    discountTypeText: "Giảm tiền",
    discountValue: 50000,
    minSpend: 0,
    maxDiscount: 0,
    quota: 1,
    used: 0,
    startsAt: "01/11/2026",
    endsAt: "30/11/2026",
    kind: VoucherKind.NEW_USER,
    ownerUserId: "u1",
    ...over,
  } as ViewVoucher;
}

function order(status: OrderStatus) {
  return { id: `o-${status}`, status } as never;
}

function setAge(ms: number | null) {
  const getMyAccount =
    ms === null
      ? vi.fn().mockRejectedValue(new Error("unavailable"))
      : vi.fn().mockResolvedValue({
          userId: "u1",
          createdAt: Timestamp.fromDate(new Date(NOW.getTime() - ms)),
        });
  vi.mocked(makeClients).mockReturnValue({ auth: { getMyAccount } } as never);
  return getMyAccount;
}

function setVouchers(list: ViewVoucher[]) {
  vi.mocked(listMyVouchers).mockResolvedValue({ ok: true, vouchers: list });
}

async function renderPanel() {
  return render(await WelcomePanel());
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers({ toFake: ["Date", "setTimeout", "clearTimeout"] });
  vi.setSystemTime(NOW);
  jar().delete(BANNER_COOKIE);
  jar().delete(CHECKLIST_COOKIE);
  vi.mocked(getPrincipal).mockReturnValue({ sub: "u1" } as never);
  setAge(2 * DAY);
  setVouchers([voucher()]);
  vi.mocked(listFollowedSellers).mockResolvedValue([]);
  vi.mocked(listAddresses).mockResolvedValue([]);
  vi.mocked(listBuyerOrders).mockResolvedValue([]);
});

afterEach(() => vi.useRealTimers());

describe("WelcomePanel banner", () => {
  it("W9 shows the NEW_USER voucher code, amount in đ and expiry", async () => {
    await renderPanel();
    expect(screen.getByText(/WELCOME50/)).toBeInTheDocument();
    expect(screen.getByText(/50\.000/)).toBeInTheDocument();
    expect(screen.getByText(/30\/11\/2026/)).toBeInTheDocument();
    expect(screen.queryByText(PENDING)).toBeNull();
  });

  it("W9 ignores a non NEW_USER voucher and picks the NEW_USER one", async () => {
    setVouchers([
      voucher({ id: "v0", code: "STANDARD10", kind: VoucherKind.STANDARD }),
      voucher(),
    ]);
    await renderPanel();
    expect(screen.getByText(/WELCOME50/)).toBeInTheDocument();
    expect(screen.queryByText(/STANDARD10/)).toBeNull();
  });

  it("W10 account 1 minute old with no NEW_USER voucher shows the pending text and no code", async () => {
    setAge(MIN);
    setVouchers([]);
    await renderPanel();
    expect(screen.getByText(PENDING)).toBeInTheDocument();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
  });

  it("W10 a STANDARD-only list counts as no NEW_USER voucher and shows the pending text", async () => {
    setAge(MIN);
    setVouchers([voucher({ code: "STANDARD10", kind: VoucherKind.STANDARD })]);
    await renderPanel();
    expect(screen.getByText(PENDING)).toBeInTheDocument();
    expect(screen.queryByText(/STANDARD10/)).toBeNull();
  });

  it("W26 pending text appears just under 1 hour of age (boundary)", async () => {
    setAge(59 * MIN);
    setVouchers([]);
    await renderPanel();
    expect(screen.getByText(PENDING)).toBeInTheDocument();
  });

  it("W26 Pending text expires with the account age (boundary)", async () => {
    setAge(2 * HOUR);
    setVouchers([]);
    await renderPanel();
    expect(screen.queryByText(PENDING)).toBeNull();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
    expect(
      screen.getByRole("link", { name: "Theo dõi một shop" }),
    ).toBeInTheDocument();
  });

  it("W11 Voucher read failed", async () => {
    setAge(MIN);
    vi.mocked(listMyVouchers).mockResolvedValue({ ok: false });
    await renderPanel();
    expect(screen.queryByText(PENDING)).toBeNull();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
    expect(
      screen.getByRole("link", { name: "Theo dõi một shop" }),
    ).toBeInTheDocument();
  });

  it("W24 Expired or used voucher", async () => {
    setAge(MIN);
    setVouchers([voucher({ endsAt: "01/11/2026" })]);
    await renderPanel();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
    expect(screen.queryByText(PENDING)).toBeNull();
  });

  it("W24 used-up NEW_USER voucher shows no code and no pending text", async () => {
    setAge(MIN);
    setVouchers([voucher({ quota: 1, used: 1 })]);
    await renderPanel();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
    expect(screen.queryByText(PENDING)).toBeNull();
  });
});

describe("WelcomePanel age window", () => {
  it("W12 renders the panel at 13 days 23 hours", async () => {
    setAge(13 * DAY + 23 * HOUR);
    await renderPanel();
    expect(screen.getByText(/WELCOME50/)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Theo dõi một shop" }),
    ).toBeInTheDocument();
  });

  it("W12 renders nothing at exactly 14 days (boundary)", async () => {
    setAge(14 * DAY);
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
  });

  it("W20 renders neither banner nor checklist for a 30 day old account even with no cookies", async () => {
    setAge(30 * DAY);
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
  });

  it("W13 renders nothing when GetMyAccount fails and does not throw", async () => {
    setAge(null);
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
  });

  it("W13 renders nothing when created_at is missing", async () => {
    vi.mocked(makeClients).mockReturnValue({
      auth: { getMyAccount: vi.fn().mockResolvedValue({ userId: "u1" }) },
    } as never);
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
  });
});

describe("WelcomePanel anonymous", () => {
  it("W15 renders nothing and issues none of the five reads for an anonymous visitor", async () => {
    vi.mocked(getPrincipal).mockReturnValue(null);
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
    expect(makeClients).not.toHaveBeenCalled();
    expect(listMyVouchers).not.toHaveBeenCalled();
    expect(listFollowedSellers).not.toHaveBeenCalled();
    expect(listAddresses).not.toHaveBeenCalled();
    expect(listBuyerOrders).not.toHaveBeenCalled();
  });
});

describe("WelcomePanel checklist", () => {
  it("W16 follow done, address and first order not done", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    await renderPanel();
    expect(screen.getByText("Theo dõi một shop")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Theo dõi một shop" }),
    ).toBeNull();
    expect(
      screen.getByRole("link", { name: "Thêm địa chỉ giao hàng" }),
    ).toHaveAttribute("href", "/account/addresses");
    expect(
      screen.getByRole("link", { name: "Đặt đơn đầu tiên" }),
    ).toHaveAttribute("href", "/");
  });

  it("W16 an undone follow step links to /account/following", async () => {
    await renderPanel();
    expect(
      screen.getByRole("link", { name: "Theo dõi một shop" }),
    ).toHaveAttribute("href", "/account/following");
  });

  it("W16 issues the five reads in parallel before any resolves", async () => {
    const gate = new Promise<never>(() => {});
    vi.mocked(listAddresses).mockReturnValue(gate);
    vi.mocked(listFollowedSellers).mockReturnValue(gate);
    vi.mocked(listBuyerOrders).mockReturnValue(gate);
    vi.mocked(listMyVouchers).mockReturnValue(gate);
    void WelcomePanel();
    await vi.advanceTimersByTimeAsync(0);
    expect(listMyVouchers).toHaveBeenCalledTimes(1);
    expect(listFollowedSellers).toHaveBeenCalledTimes(1);
    expect(listAddresses).toHaveBeenCalledTimes(1);
    expect(listBuyerOrders).toHaveBeenCalledTimes(1);
  });

  it("W17 all three done renders no checklist and no hide control", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    vi.mocked(listAddresses).mockResolvedValue([{ id: "a1" }] as never);
    vi.mocked(listBuyerOrders).mockResolvedValue([order(OrderStatus.PENDING)]);
    await renderPanel();
    expect(screen.queryByText("Theo dõi một shop")).toBeNull();
    expect(screen.queryByText("Thêm địa chỉ giao hàng")).toBeNull();
    expect(screen.queryByText("Đặt đơn đầu tiên")).toBeNull();
    expect(screen.queryByRole("button", { name: "Ẩn danh sách" })).toBeNull();
  });

  it("W18 One read fails", async () => {
    vi.mocked(listAddresses).mockResolvedValue([]);
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    vi.mocked(listBuyerOrders).mockResolvedValue([
      order(OrderStatus.COMPLETED),
    ]);
    await renderPanel();
    expect(
      screen.getByRole("link", { name: "Thêm địa chỉ giao hàng" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Theo dõi một shop" }),
    ).toBeNull();
    expect(screen.queryByRole("link", { name: "Đặt đơn đầu tiên" })).toBeNull();
    expect(screen.getByText("Theo dõi một shop")).toBeInTheDocument();
    expect(screen.getByText("Đặt đơn đầu tiên")).toBeInTheDocument();
  });

  it("W25 only cancelled orders leave the first-order step not done", async () => {
    vi.mocked(listBuyerOrders).mockResolvedValue([
      order(OrderStatus.CANCELLED),
      order(OrderStatus.CANCELLED),
    ]);
    await renderPanel();
    expect(
      screen.getByRole("link", { name: "Đặt đơn đầu tiên" }),
    ).toBeInTheDocument();
  });

  it("W25 a cancelled order next to a pending one ticks the first-order step", async () => {
    vi.mocked(listBuyerOrders).mockResolvedValue([
      order(OrderStatus.CANCELLED),
      order(OrderStatus.PENDING),
    ]);
    await renderPanel();
    expect(screen.queryByRole("link", { name: "Đặt đơn đầu tiên" })).toBeNull();
    expect(screen.getByText("Đặt đơn đầu tiên")).toBeInTheDocument();
  });

  it("W21 Slow read times out at 2 s", async () => {
    vi.mocked(listAddresses).mockReturnValue(new Promise<never>(() => {}));
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    const pending = WelcomePanel();
    await vi.advanceTimersByTimeAsync(2500);
    render(await pending);
    expect(
      screen.getByRole("link", { name: "Thêm địa chỉ giao hàng" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Theo dõi một shop" }),
    ).toBeNull();
    expect(screen.getByText(/WELCOME50/)).toBeInTheDocument();
  });
});

describe("WelcomePanel dismissal", () => {
  it("W14 the banner cookie hides the banner and keeps the checklist", async () => {
    jar().set(BANNER_COOKIE, "1");
    await renderPanel();
    expect(screen.queryByText(/WELCOME50/)).toBeNull();
    expect(screen.queryByText(PENDING)).toBeNull();
    expect(
      screen.getByRole("link", { name: "Theo dõi một shop" }),
    ).toBeInTheDocument();
  });

  it("W19 the checklist cookie hides the checklist and keeps the banner", async () => {
    jar().set(CHECKLIST_COOKIE, "1");
    await renderPanel();
    expect(screen.getByText(/WELCOME50/)).toBeInTheDocument();
    expect(screen.queryByText("Theo dõi một shop")).toBeNull();
    expect(screen.queryByRole("button", { name: "Ẩn danh sách" })).toBeNull();
  });

  it("W14 both cookies leave an in-window account with an empty panel", async () => {
    jar().set(BANNER_COOKIE, "1");
    jar().set(CHECKLIST_COOKIE, "1");
    const { container } = await renderPanel();
    expect(container).toBeEmptyDOMElement();
  });

  it("W14 the banner close control calls dismissWelcome with banner", async () => {
    vi.useRealTimers();
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
    const user = setupUser();
    await renderPanel();
    await user.click(screen.getByRole("button", { name: /đóng/i }));
    expect(dismissWelcome).toHaveBeenCalledWith("banner");
  });

  it("W19 the Ẩn danh sách control calls dismissWelcome with checklist", async () => {
    vi.useRealTimers();
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(NOW);
    const user = setupUser();
    await renderPanel();
    await user.click(screen.getByRole("button", { name: "Ẩn danh sách" }));
    expect(dismissWelcome).toHaveBeenCalledWith("checklist");
  });
});

describe("WelcomePanel markup contract", () => {
  it("W9 renders the banner in welcome-banner with a close button named Đóng", async () => {
    await renderPanel();
    const banner = screen.getByTestId("welcome-banner");
    expect(within(banner).getByText(/WELCOME50/)).toBeInTheDocument();
    expect(
      within(banner).getByRole("button", { name: /Đóng/i }),
    ).toBeInTheDocument();
  });

  it("W16 renders welcome-checklist with one welcome-step per step carrying data-done", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    await renderPanel();
    const checklist = screen.getByTestId("welcome-checklist");
    const steps = within(checklist).getAllByTestId("welcome-step");
    expect(steps.map((s) => s.getAttribute("data-done"))).toEqual([
      "true",
      "false",
      "false",
    ]);
    expect(
      within(checklist).getByRole("button", { name: "Ẩn danh sách" }),
    ).toBeInTheDocument();
  });

  it("W17 renders no welcome-checklist when all steps are done", async () => {
    vi.mocked(listFollowedSellers).mockResolvedValue([
      { sellerId: "s1", displayName: "Shop" },
    ]);
    vi.mocked(listAddresses).mockResolvedValue([{ id: "a1" }] as never);
    vi.mocked(listBuyerOrders).mockResolvedValue([order(OrderStatus.PENDING)]);
    await renderPanel();
    expect(screen.queryByTestId("welcome-checklist")).toBeNull();
  });
});
