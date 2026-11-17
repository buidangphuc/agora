import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { getOrder } from "@/lib/gateway/orders";

const state = vi.hoisted(() => ({
  principal: null as null | { id: string; name: string; scopes: string[] },
}));
const redirectGuest = vi.hoisted(() =>
  vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
);

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/seller/orders/print",
  redirect: vi.fn(() => {
    throw new Error("NEXT_REDIRECT");
  }),
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));
vi.mock("@/features/auth/guardPath", () => ({
  redirectGuestToLogin: redirectGuest,
  redirectGuestToLoginFromHeader: redirectGuest,
}));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: () => state.principal,
  hasScope: (s: string) => state.principal?.scopes.includes(s) ?? false,
  hasSellerAccess: (p: { scopes: string[] } | null) =>
    p?.scopes.includes("listing.write") ?? false,
}));
vi.mock("@/lib/gateway/orders", () => ({
  getOrder: vi.fn(),
  getShipmentTracking: vi.fn(),
  listSellerOrders: vi.fn(),
}));
vi.mock("@/features/seller/actions", () => ({ shipOrderAction: vi.fn() }));

import * as printModule from "./page";
const PrintPage = printModule.default;

function order(id: string) {
  return {
    id,
    buyerId: "b",
    sellerId: "me",
    status: OrderStatus.PAID,
    statusText: "Đã thanh toán",
    totalAmount: 250000,
    itemsSubtotal: 220000,
    shippingFee: 30000,
    paymentMethod: 0,
    paymentMethodText: "COD",
    currency: "VND",
    discountAmount: 0,
    voucherCode: "",
    recipientName: `Khách ${id}`,
    phone: `090${id}`,
    addressFull: `Địa chỉ ${id}`,
    trackingNumber: `TRK-${id}`,
    items: [
      {
        id: `it-${id}`,
        title: `Sản phẩm ${id}`,
        variantName: "Đỏ",
        quantity: 2,
        unitPrice: 110000,
        imageUrl: "",
      },
    ],
    createdAt: "01/10/2026",
    paidAt: "",
    paymentDueAt: "",
    shipBy: "",
    shippedAt: "",
    completedAt: "",
    returnWindowEndsAt: "",
    autoCompleteAt: "",
    cancelReason: 0,
  };
}

async function renderPage(ids?: string) {
  const ui = await PrintPage({
    searchParams: Promise.resolve(ids === undefined ? {} : { ids }),
  } as never);
  return render(ui);
}

beforeEach(() => {
  vi.clearAllMocks();
  state.principal = { id: "me", name: "Me", scopes: ["listing.write"] };
  vi.mocked(getOrder).mockImplementation((async (id: string) =>
    order(id)) as never);
});

describe("Bulk ship: three selected orders print three slips", () => {
  it("renders one slip per order in id order", async () => {
    await renderPage("a1,b2,c3");
    const slips = screen.getAllByTestId("packing-slip");
    expect(slips).toHaveLength(3);
    ["a1", "b2", "c3"].forEach((id, i) => {
      const s = within(slips[i]);
      expect(s.getByText(new RegExp(`Khách ${id}`))).toBeTruthy();
      expect(s.getByText(new RegExp(`090${id}`))).toBeTruthy();
      expect(s.getByText(new RegExp(`Địa chỉ ${id}`))).toBeTruthy();
      expect(s.getByText(new RegExp(`Sản phẩm ${id}`))).toBeTruthy();
      expect(s.getByText(new RegExp(`TRK-${id}`))).toBeTruthy();
    });
    expect(getOrder).toHaveBeenCalledTimes(3);
  });
});

describe("Bulk ship: an unreadable order is named and skipped", () => {
  it("unreadable order named not printed", async () => {
    vi.mocked(getOrder).mockImplementation((async (id: string) =>
      id === "b2" ? null : order(id)) as never);
    await renderPage("a1,b2,c3");
    expect(screen.getAllByTestId("packing-slip")).toHaveLength(2);
    const notice = screen.getByText(/Không in được/);
    expect(notice.textContent).toContain("b2");
    expect(notice.textContent).not.toContain("a1");
    expect(notice.closest("[data-print-hidden]")).not.toBeNull();
  });
});

describe("Bulk ship: more than 50 print ids are rejected", () => {
  it("51 ids rejected without calls", async () => {
    const ids = Array.from({ length: 51 }, (_, i) => `o${i}`).join(",");
    await renderPage(ids);
    expect(screen.queryAllByTestId("packing-slip")).toHaveLength(0);
    expect(screen.getByText(/Chọn tối đa 50 đơn/)).toBeTruthy();
    expect(getOrder).not.toHaveBeenCalled();
  });
});

describe("Bulk ship: an anonymous visitor or a non-seller cannot print", () => {
  it("no slips and no calls without seller scope", async () => {
    state.principal = null;
    await expect(renderPage("a1,b2")).rejects.toThrow();
    expect(redirectGuest).toHaveBeenCalled();
    expect(getOrder).not.toHaveBeenCalled();

    state.principal = { id: "u", name: "U", scopes: [] };
    try {
      await renderPage("a1,b2");
    } catch {
      // a denial thrown as notFound/redirect is also acceptable
    }
    expect(screen.queryAllByTestId("packing-slip")).toHaveLength(0);
    expect(getOrder).not.toHaveBeenCalled();
  });
});

describe("Bulk ship: the print page is not cached or indexed", () => {
  it("no-store and noindex", () => {
    expect(printModule.dynamic).toBe("force-dynamic");
    const robots = (printModule as { metadata?: { robots?: unknown } }).metadata
      ?.robots as { index?: boolean } | string | undefined;
    const noindex =
      typeof robots === "string"
        ? robots.includes("noindex")
        : robots?.index === false;
    expect(noindex).toBe(true);
  });
});
