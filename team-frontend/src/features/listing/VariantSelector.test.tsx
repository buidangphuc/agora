import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { PurchaseProvider } from "@/features/cart/PurchaseContext";
import { usePurchase } from "@/features/cart/PurchaseContext";
import type { ViewVariant } from "@/lib/gateway/listings";
import { setupUser } from "@/test/user";
import { VariantSelector } from "./VariantSelector";

const replace = vi.fn();
let search = "";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
  usePathname: () => "/listing/l1",
  useSearchParams: () => new URLSearchParams(search),
}));
vi.mock("@/features/cart/actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

function v(over: Partial<ViewVariant>): ViewVariant {
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

const variants = [
  v({ id: "base", name: "128GB", price: 100, stock: 10 }),
  v({ id: "same", name: "128GB Đen", price: 0, stock: 10 }),
  v({ id: "big", name: "256GB", price: 150, stock: 4 }),
  v({ id: "gone", name: "512GB", price: 200, stock: 0 }),
];
const listing = {
  id: "l1",
  title: "Phone",
  categoryId: "c",
  price: 100,
  stock: 10,
  currency: "VND",
  variants,
};

function Quantity() {
  const { quantity, setQuantity } = usePurchase();
  return (
    <button type="button" onClick={() => setQuantity(3)}>
      qty:{quantity}
    </button>
  );
}

function renderSelector(initial = "base") {
  return render(
    <PurchaseProvider listing={listing} initialVariantId={initial}>
      <VariantSelector />
      <Quantity />
    </PurchaseProvider>,
  );
}

beforeEach(() => {
  replace.mockClear();
  search = "";
});

describe("VariantSelector", () => {
  it("writes ?variant= with replace and no scroll when price or stock differs", async () => {
    const user = setupUser();
    renderSelector();
    await user.click(screen.getByRole("radio", { name: "256GB" }));
    expect(replace).toHaveBeenCalledTimes(1);
    expect(replace).toHaveBeenCalledWith("/listing/l1?variant=big", {
      scroll: false,
    });
    expect(screen.getByRole("radio", { name: "256GB" })).toBeChecked();
  });

  it("keeps other search params when it writes", async () => {
    const user = setupUser();
    search = "rating=4&rpage=2";
    renderSelector();
    await user.click(screen.getByRole("radio", { name: "256GB" }));
    expect(replace).toHaveBeenCalledWith(
      "/listing/l1?rating=4&rpage=2&variant=big",
      { scroll: false },
    );
  });

  it("does not write when price and stock equal the base", async () => {
    const user = setupUser();
    renderSelector("big");
    await user.click(screen.getByRole("radio", { name: "128GB Đen" }));
    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByRole("radio", { name: "128GB Đen" })).toBeChecked();
  });

  it("keeps the URL in step once it already carries a variant", async () => {
    const user = setupUser();
    search = "variant=big";
    renderSelector("big");
    await user.click(screen.getByRole("radio", { name: "128GB Đen" }));
    expect(replace).toHaveBeenCalledWith("/listing/l1?variant=same", {
      scroll: false,
    });
  });

  it("disables an out-of-stock variant with an Hết hàng tag", async () => {
    const user = setupUser();
    renderSelector();
    const gone = screen.getByRole("radio", { name: /512GB/ });
    expect(gone).toBeDisabled();
    expect(gone).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText("Hết hàng")).toBeInTheDocument();
    await user.click(gone);
    expect(gone).not.toBeChecked();
    expect(replace).not.toHaveBeenCalled();
  });

  it("resets the quantity to 1 when the variant changes", async () => {
    const user = setupUser();
    renderSelector();
    await user.click(screen.getByRole("button", { name: /qty:/ }));
    expect(screen.getByRole("button", { name: "qty:3" })).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "256GB" }));
    expect(screen.getByRole("button", { name: "qty:1" })).toBeInTheDocument();
  });

  it("renders nothing for a listing without variants", () => {
    const { container } = render(
      <PurchaseProvider
        listing={{ ...listing, variants: [] }}
        initialVariantId=""
      >
        <VariantSelector />
      </PurchaseProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
