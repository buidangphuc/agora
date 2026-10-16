import { redirect } from "next/navigation";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { getOrderResult } from "@/lib/gateway/orders";
import { getPrincipal } from "@/lib/gateway/session";

import BuyerOrderLayout from "./layout";
import BuyerOrderDetailPage from "./page";

vi.mock("@/lib/gateway/orders", () => ({
  getOrderResult: vi.fn(),
  getShipmentTracking: vi.fn(),
  getSagaState: vi.fn(),
  listOrderReturns: vi.fn(),
}));
vi.mock("@/lib/gateway/session", () => ({ getPrincipal: vi.fn() }));
vi.mock("next/navigation", () => ({
  redirect: vi.fn((url: string) => {
    throw new Error(`NEXT_REDIRECT:${url}`);
  }),
  notFound: vi.fn(),
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));
vi.mock("@/features/order/actions", () => ({
  reorderAction: vi.fn(),
  cancelOrderAction: vi.fn(),
  createReturnRequestAction: vi.fn(),
}));

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue(null);
});

describe("/account/orders/[id] guards", () => {
  it("[Guest opening a protected page is sent to login with its path] redirects to login with returnUrl (page)", async () => {
    await expect(
      BuyerOrderDetailPage({
        params: Promise.resolve({ id: "o-9" }),
        searchParams: Promise.resolve({}),
      }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Forders%2Fo-9",
    );
    expect(getOrderResult).not.toHaveBeenCalled();
  });

  it("[Query string of the guarded page is preserved] page keeps tab query", async () => {
    await expect(
      BuyerOrderDetailPage({
        params: Promise.resolve({ id: "o-9" }),
        searchParams: Promise.resolve({ tab: "returns" }),
      }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Forders%2Fo-9%3Ftab%3Dreturns",
    );
  });

  it("[Guest opening a protected page is sent to login with its path] re-encodes the id param (page)", async () => {
    await expect(
      BuyerOrderDetailPage({
        params: Promise.resolve({ id: "a b" }),
        searchParams: Promise.resolve({}),
      }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Forders%2Fa%2520b",
    );
  });

  it("[Guest opening a protected page is sent to login with its path] redirects to login with returnUrl (layout)", async () => {
    await expect(
      BuyerOrderLayout({
        children: null,
        params: Promise.resolve({ id: "o-9" }),
      }),
    ).rejects.toThrow("NEXT_REDIRECT");
    expect(redirect).toHaveBeenCalledWith(
      "/login?returnUrl=%2Faccount%2Forders%2Fo-9",
    );
    expect(getOrderResult).not.toHaveBeenCalled();
  });
});
