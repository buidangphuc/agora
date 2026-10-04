import { act, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ToastProvider } from "@/components/ui/ToastProvider";
import { trackEcommerce } from "@/lib/analytics";
import type { ViewVariant } from "@/lib/gateway/listings";
import { setupUser } from "@/test/user";
import { BuyBar } from "./BuyBar";
import { type PurchaseListing, PurchaseProvider } from "./PurchaseContext";
import { PurchasePanel } from "./PurchasePanel";
import { addToCartAction } from "./actions";

const push = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
}));
vi.mock("./actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

function variant(over: Partial<ViewVariant>): ViewVariant {
  return {
    id: "v",
    listingId: "l1",
    name: "V",
    sku: "",
    price: 0,
    stock: 10,
    imageUrl: "",
    ...over,
  };
}

const baseListing: PurchaseListing = {
  id: "l1",
  title: "Điện thoại",
  categoryId: "cat1",
  price: 100000,
  stock: 3,
  currency: "VND",
  variants: [],
};

function renderPanel(listing: PurchaseListing = baseListing, initial = "") {
  return render(
    <ToastProvider>
      <PurchaseProvider listing={listing} initialVariantId={initial}>
        <PurchasePanel />
        <BuyBar />
      </PurchaseProvider>
    </ToastProvider>,
  );
}

const inlineAdd = () => screen.getByTestId("pdp-add-to-cart");
const inlineBuy = () => screen.getByTestId("pdp-buy-now");

/** A promise resolved from the test, to observe the pending state. */
function deferred<T>() {
  let resolve: (v: T) => void = () => {};
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PurchasePanel", () => {
  it("is pending (both buttons disabled, clicked one busy) then shows a success toast and fires add_to_cart once", async () => {
    const user = setupUser();
    const d = deferred<{ ok: true }>();
    vi.mocked(addToCartAction).mockReturnValue(d.promise);
    renderPanel();

    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    await user.click(inlineAdd());

    expect(addToCartAction).toHaveBeenCalledWith("l1", undefined, 2);
    expect(inlineAdd()).toBeDisabled();
    expect(inlineAdd()).toHaveAttribute("aria-busy", "true");
    expect(inlineBuy()).toBeDisabled();
    expect(inlineBuy()).not.toHaveAttribute("aria-busy");

    await act(async () => {
      d.resolve({ ok: true });
    });

    expect(
      await screen.findByText("Đã thêm 2 sản phẩm vào giỏ hàng"),
    ).toBeInTheDocument();
    expect(inlineAdd()).toBeEnabled();
    expect(inlineBuy()).toBeEnabled();
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("add_to_cart", {
      currency: "VND",
      value: 200000,
      items: [
        {
          itemId: "l1",
          itemName: "Điện thoại",
          price: 100000,
          quantity: 2,
          itemCategory: "cat1",
        },
      ],
    });
  });

  it("reports a failure with an error toast, fires no event and re-enables the buttons", async () => {
    const user = setupUser();
    // The cart phase migrates addToCartAction to the shared ActionResult.
    vi.mocked(addToCartAction).mockResolvedValue({
      ok: false,
      error: "Hết hàng rồi",
    } as never);
    renderPanel();
    await user.click(inlineAdd());
    expect(await screen.findByText("Hết hàng rồi")).toBeInTheDocument();
    expect(trackEcommerce).not.toHaveBeenCalled();
    expect(inlineAdd()).toBeEnabled();
    expect(inlineBuy()).toBeEnabled();
  });

  it("falls back to the legacy message field and to a generic text on a throw", async () => {
    const user = setupUser();
    vi.mocked(addToCartAction).mockResolvedValueOnce({
      ok: false,
      message: "Lỗi cũ",
    } as never);
    renderPanel();
    await user.click(inlineAdd());
    expect(await screen.findByText("Lỗi cũ")).toBeInTheDocument();

    vi.mocked(addToCartAction).mockRejectedValueOnce(new Error("network"));
    await user.click(inlineAdd());
    expect(
      await screen.findByText("Có lỗi xảy ra khi thêm vào giỏ hàng."),
    ).toBeInTheDocument();
    expect(inlineAdd()).toBeEnabled();
  });

  it("cannot exceed the stock of the selected variant", async () => {
    const user = setupUser();
    renderPanel(); // stock 3
    const plus = screen.getByRole("button", { name: "Tăng số lượng" });
    for (let i = 0; i < 4; i++) await user.click(plus);
    expect(screen.getByRole("spinbutton", { name: "Số lượng" })).toHaveValue(
      "3",
    );
    expect(plus).toBeDisabled();
  });

  it("Mua ngay adds the selected variant once and goes to /checkout", async () => {
    const user = setupUser();
    vi.mocked(addToCartAction).mockResolvedValue({ ok: true });
    const listing = {
      ...baseListing,
      variants: [
        variant({ id: "a", name: "128GB", price: 100000, stock: 3 }),
        variant({ id: "b", name: "256GB", price: 150000, stock: 5 }),
      ],
    };
    renderPanel(listing, "b");
    await user.click(inlineBuy());
    expect(addToCartAction).toHaveBeenCalledWith("l1", "b", 1);
    expect(push).toHaveBeenCalledWith("/checkout");
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith(
      "add_to_cart",
      expect.objectContaining({ value: 150000 }),
    );
  });

  it("an out-of-stock variant disables both actions, the picker, and warns", () => {
    const listing = {
      ...baseListing,
      variants: [variant({ id: "a", name: "128GB", stock: 0 })],
    };
    renderPanel(listing, "a");
    expect(inlineAdd()).toBeDisabled();
    expect(inlineBuy()).toBeDisabled();
    expect(screen.getByRole("spinbutton", { name: "Số lượng" })).toBeDisabled();
    expect(screen.getByText("Phân loại này đã hết hàng")).toBeInTheDocument();
  });
});

describe("BuyBar", () => {
  it("is a below-lg bar and the inline row is hidden below lg: one visible set per breakpoint", () => {
    renderPanel();
    const bar = screen.getByTestId("buy-bar");
    expect(bar).toHaveClass("fixed", "bottom-0", "lg:hidden");
    expect(inlineAdd().parentElement).toHaveClass("hidden", "lg:flex");
    // Both rows exist in the DOM, but each one is shown at one breakpoint only.
    expect(
      within(bar).getByRole("button", { name: "Thêm vào giỏ" }),
    ).toBeInTheDocument();
    expect(
      within(bar).getByRole("button", { name: "Mua ngay" }),
    ).toBeInTheDocument();
  });

  it("reads the selected variant's price and the flash-sale price when there is one", () => {
    const listing = {
      ...baseListing,
      variants: [
        variant({ id: "a", name: "128GB", price: 100000, stock: 3 }),
        variant({ id: "b", name: "256GB", price: 150000, stock: 5 }),
      ],
    };
    const { unmount } = renderPanel(listing, "b");
    expect(
      within(screen.getByTestId("buy-bar")).getByText("150.000"),
    ).toBeInTheDocument();
    unmount();
    renderPanel(
      { ...listing, flashSale: { variantId: "", salePrice: 90000 } },
      "b",
    );
    expect(
      within(screen.getByTestId("buy-bar")).getByText("90.000"),
    ).toBeInTheDocument();
  });

  it("shares the pending state with the inline panel and adds the same way", async () => {
    const user = setupUser();
    const d = deferred<{ ok: true }>();
    vi.mocked(addToCartAction).mockReturnValue(d.promise);
    renderPanel();
    const bar = screen.getByTestId("buy-bar");
    await user.click(within(bar).getByRole("button", { name: "Thêm vào giỏ" }));
    expect(within(bar).getByTestId("bar-add-to-cart")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(inlineAdd()).toBeDisabled();
    expect(inlineBuy()).toBeDisabled();
    await act(async () => {
      d.resolve({ ok: true });
    });
    expect(
      await screen.findByText("Đã thêm 1 sản phẩm vào giỏ hàng"),
    ).toBeInTheDocument();
  });
});
