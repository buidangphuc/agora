import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const listVouchers = vi.hoisted(() => vi.fn());
const listMyVouchers = vi.hoisted(() => vi.fn());
const getPrincipal = vi.hoisted(() => vi.fn());
const hasScope = vi.hoisted(() => vi.fn());

vi.mock("@/lib/gateway/promotion", async () => {
  const actual = await vi.importActual<
    typeof import("@/generated/platform/promotion/v1/promotion_pb.js")
  >("@/generated/platform/promotion/v1/promotion_pb.js");
  return {
    listVouchers,
    listMyVouchers,
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
  kind: 1,
  ownerUserId: "",
};

const own = {
  ...fixed,
  id: "v9",
  code: "WELCOME50",
  scopeText: "Toàn sàn",
  scope: 2,
  discountType: 2,
  discountValue: 70000,
  minSpend: 200000,
  endsAt: "30/11/2026",
  kind: 2,
  ownerUserId: "u1",
};

beforeEach(() => {
  vi.clearAllMocks();
  listVouchers.mockResolvedValue([fixed]);
  listMyVouchers.mockResolvedValue({ ok: true, vouchers: [] });
  getPrincipal.mockReturnValue(null);
  hasScope.mockReturnValue(false);
});

describe("VouchersPage", () => {
  it("renders the real vouchers filtered by ?type=", async () => {
    render(
      await VouchersPage({
        searchParams: Promise.resolve({ type: "percent" }),
      }),
    );
    expect(screen.queryByTestId("voucher-card")).toBeNull();
    render(
      await VouchersPage({ searchParams: Promise.resolve({ type: "fixed" }) }),
    );
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
  });

  it("falls back to all for an unknown type", async () => {
    render(
      await VouchersPage({ searchParams: Promise.resolve({ type: "zzz" }) }),
    );
    expect(screen.getByTestId("voucher-card")).toBeInTheDocument();
  });

  it("hides the seller manager without the listing.write scope", async () => {
    getPrincipal.mockReturnValue({ sub: "u" });
    hasScope.mockReturnValue(false);
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.queryByText("MANAGER")).toBeNull();
  });

  it("hides the seller manager for anonymous visitors", async () => {
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.queryByText("MANAGER")).toBeNull();
  });

  it("shows the seller manager with listing.write", async () => {
    getPrincipal.mockReturnValue({ sub: "u" });
    hasScope.mockImplementation((s: string) => s === "listing.write");
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("MANAGER")).toBeInTheDocument();
  });

  it("renders Empty when the gateway returned nothing", async () => {
    listVouchers.mockResolvedValue([]);
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(
      screen.getByRole("link", { name: "Xem sản phẩm" }),
    ).toBeInTheDocument();
  });
});

describe("VouchersPage own vouchers", () => {
  const HEADING = "Voucher của bạn";

  it("W1 renders Voucher của bạn with the code, amount, minimum spend and expiry before the platform list", async () => {
    getPrincipal.mockReturnValue({ sub: "u1" });
    listMyVouchers.mockResolvedValue({ ok: true, vouchers: [own] });
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    const heading = screen.getByRole("heading", { name: HEADING });
    const code = screen.getByText("WELCOME50");
    expect(code).toBeInTheDocument();
    expect(screen.getByText(/70\.000/, { selector: "h3" })).toBeInTheDocument();
    expect(screen.getByText(/200\.000/)).toBeInTheDocument();
    expect(screen.getByText(/30\/11\/2026/)).toBeInTheDocument();
    const platform = screen
      .getAllByTestId("voucher-card")
      .find((el) => el.getAttribute("data-code") === "MINUS50K") as HTMLElement;
    expect(
      heading.compareDocumentPosition(platform) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      code.compareDocumentPosition(platform) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("W2 No own vouchers (boundary)", async () => {
    getPrincipal.mockReturnValue({ sub: "u1" });
    listMyVouchers.mockResolvedValue({ ok: true, vouchers: [] });
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.queryByRole("heading", { name: HEADING })).toBeNull();
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
  });

  it("W3 Read failure keeps the page", async () => {
    getPrincipal.mockReturnValue({ sub: "u1" });
    listMyVouchers.mockResolvedValue({ ok: false });
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(listMyVouchers).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("heading", { name: HEADING })).toBeNull();
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
  });

  it("W4 an anonymous visitor sees the platform list and no Voucher của bạn section", async () => {
    getPrincipal.mockReturnValue(null);
    listMyVouchers.mockResolvedValue({ ok: true, vouchers: [] });
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.queryByRole("heading", { name: HEADING })).toBeNull();
    expect(screen.getByTestId("voucher-card")).toHaveAttribute(
      "data-code",
      "MINUS50K",
    );
  });

  it("W5 buyer A sees their own code and buyer B, whose read is empty, never does", async () => {
    getPrincipal.mockReturnValue({ sub: "a" });
    listMyVouchers.mockResolvedValue({ ok: true, vouchers: [own] });
    const a = render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.getByText("WELCOME50")).toBeInTheDocument();
    a.unmount();

    getPrincipal.mockReturnValue({ sub: "b" });
    listMyVouchers.mockResolvedValue({ ok: true, vouchers: [] });
    render(await VouchersPage({ searchParams: Promise.resolve({}) }));
    expect(screen.queryByText("WELCOME50")).toBeNull();
  });
});
