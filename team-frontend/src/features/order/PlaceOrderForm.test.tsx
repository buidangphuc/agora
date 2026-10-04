import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PaymentMethod } from "@/generated/platform/payment/v1/payment_pb.js";
import { trackEcommerce } from "@/lib/analytics";
import { setupUser } from "@/test/user";

import { CheckoutPendingProvider } from "./CheckoutPending";
import {
  MobilePlaceOrderButton,
  PlaceOrderForm,
  type PlaceOrderFormProps,
} from "./PlaceOrderForm";
import { checkoutAction } from "./actions";

const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));
const nav = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: nav.push }),
}));
vi.mock("./actions", () => ({ checkoutAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

type Res = Awaited<ReturnType<typeof checkoutAction>>;

function deferred() {
  let resolve!: (v: Res) => void;
  const promise = new Promise<Res>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

const props: PlaceOrderFormProps = {
  addressId: "a1",
  method: PaymentMethod.MOCK_MOMO,
  voucherCode: "SAVE10",
  total: 290000,
  shippingTier: "STANDARD",
  items: [
    { itemId: "l1", itemName: "Áo", price: 100000, quantity: 3, index: 1 },
  ],
  backHref: "/checkout?step=payment",
};

function setup() {
  return render(
    <CheckoutPendingProvider>
      <PlaceOrderForm {...props} />
      <MobilePlaceOrderButton />
    </CheckoutPendingProvider>,
  );
}

beforeEach(() => vi.clearAllMocks());

describe("PlaceOrderForm double-submit guard", () => {
  it("a same-tick double click calls checkoutAction exactly once and tracks one purchase", async () => {
    const d = deferred();
    vi.mocked(checkoutAction).mockReturnValue(d.promise);
    setup();
    const btn = screen.getAllByRole("button", {
      name: "Đặt hàng",
    })[0] as HTMLElement;

    // Two activations before React can re-render the button as disabled.
    act(() => {
      fireEvent.click(btn);
      fireEvent.click(btn);
    });
    expect(checkoutAction).toHaveBeenCalledTimes(1);
    expect(checkoutAction).toHaveBeenCalledWith(
      "a1",
      undefined,
      PaymentMethod.MOCK_MOMO,
      "SAVE10",
    );

    await act(async () => {
      d.resolve({
        ok: true,
        data: { orderIds: ["o1"], paymentUrl: "/pay/o1" },
      });
      await d.promise;
    });
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("purchase", {
      transactionId: "o1",
      currency: "VND",
      value: 290000,
      coupon: "SAVE10",
      shippingTier: "STANDARD",
      paymentType: String(PaymentMethod.MOCK_MOMO),
      items: props.items,
    });
    expect(nav.push).toHaveBeenCalledWith("/pay/o1");
  });

  it("is disabled, aria-busy and keeps its width while pending; Back is inert", async () => {
    const user = setupUser();
    const d = deferred();
    vi.mocked(checkoutAction).mockReturnValue(d.promise);
    setup();
    expect(screen.getByRole("link", { name: "Quay lại" })).toHaveAttribute(
      "href",
      "/checkout?step=payment",
    );
    await user.click(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0] as HTMLElement,
    );

    const btn = screen.getAllByRole("button", {
      name: "Đặt hàng",
    })[0] as HTMLElement;
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("aria-busy", "true");
    expect(btn).toHaveAttribute("aria-disabled", "true");
    expect(btn.className).toContain("min-w-40");
    expect(screen.queryByRole("link", { name: "Quay lại" })).toBeNull();
    expect(screen.getByText("Quay lại")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    // the mobile bar button is locked too
    expect(
      screen.getAllByRole("button", { name: "Đặt hàng" })[1],
    ).toBeDisabled();

    await act(async () => {
      d.resolve({ ok: false, error: "x" });
      await d.promise;
    });
  });

  it("Enter in the form cannot bypass the guard while pending", async () => {
    const d = deferred();
    vi.mocked(checkoutAction).mockReturnValue(d.promise);
    const { container } = setup();
    const form = container.querySelector("form") as HTMLFormElement;
    act(() => {
      fireEvent.submit(form);
    });
    act(() => {
      fireEvent.submit(form);
      fireEvent.submit(form);
    });
    expect(checkoutAction).toHaveBeenCalledTimes(1);
    await act(async () => {
      d.resolve({ ok: false, error: "x" });
      await d.promise;
    });
  });

  it("stays locked after success: no further call can be made", async () => {
    const d = deferred();
    vi.mocked(checkoutAction).mockReturnValue(d.promise);
    const { container } = setup();
    const form = container.querySelector("form") as HTMLFormElement;
    act(() => {
      fireEvent.submit(form);
    });
    await act(async () => {
      d.resolve({ ok: true, data: { orderIds: ["o1"] } });
      await d.promise;
    });
    const btn = screen.getAllByRole("button", {
      name: "Đặt hàng",
    })[0] as HTMLElement;
    expect(btn).toBeDisabled();
    act(() => {
      fireEvent.submit(form);
    });
    expect(checkoutAction).toHaveBeenCalledTimes(1);
    expect(nav.push).toHaveBeenCalledWith("/account/orders?success=1");
  });

  it("is re-enabled after a failure and shows the saga Alert with recovery actions", async () => {
    const user = setupUser();
    vi.mocked(checkoutAction).mockResolvedValue({
      ok: false,
      error: "Sản phẩm đã hết hàng",
    });
    setup();
    await user.click(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0] as HTMLElement,
    );

    const btn = screen.getAllByRole("button", {
      name: "Đặt hàng",
    })[0] as HTMLElement;
    expect(btn).toBeEnabled();
    const slot = screen.getByTestId("saga-alert-slot");
    const alert = screen.getByRole("alert");
    expect(slot).toContainElement(alert);
    expect(alert).toHaveTextContent("Sản phẩm đã hết hàng");
    expect(
      screen.getByRole("link", { name: "Quay lại giỏ hàng" }),
    ).toHaveAttribute("href", "/cart");
    expect(toast.error).toHaveBeenCalledWith("Sản phẩm đã hết hàng");
    expect(trackEcommerce).not.toHaveBeenCalled();
    expect(screen.getByRole("link", { name: "Quay lại" })).toBeInTheDocument();

    // "Thử lại" clears the alert and the order can be placed again.
    await user.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(screen.queryByRole("alert")).toBeNull();
    vi.mocked(checkoutAction).mockResolvedValue({
      ok: true,
      data: { orderIds: ["o2"] },
    });
    await user.click(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0] as HTMLElement,
    );
    expect(checkoutAction).toHaveBeenCalledTimes(2);
  });

  it("releases the lock when the action throws", async () => {
    const user = setupUser();
    vi.mocked(checkoutAction).mockRejectedValue(new Error("network down"));
    setup();
    await user.click(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0] as HTMLElement,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("network down");
    expect(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0],
    ).toBeEnabled();
  });

  it("keeps the Alert slot in place so the CTA does not move", async () => {
    const user = setupUser();
    vi.mocked(checkoutAction).mockResolvedValue({ ok: false, error: "x" });
    setup();
    const slot = screen.getByTestId("saga-alert-slot");
    expect(slot.className).toContain("min-h-");
    const before = slot.nextElementSibling;
    await user.click(
      screen.getAllByRole("button", { name: "Đặt hàng" })[0] as HTMLElement,
    );
    expect(screen.getByTestId("saga-alert-slot").nextElementSibling).toBe(
      before,
    );
  });
});
