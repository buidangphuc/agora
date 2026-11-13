import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/features/cart/actions", () => ({ addBundleToCartAction: vi.fn() }));

import { BundleCard } from "./BundleCard";

const bundle = {
  id: "b1",
  sellerId: "s1",
  title: "Combo hè",
  listingIds: ["l1", "l2"],
  bundlePrice: 120000,
  createdAt: "",
};
const l1 = { id: "l1", title: "Áo thun", price: 100000 };
const l2 = { id: "l2", title: "Quần short", price: 50000 };

describe("BundleCard", () => {
  it("[C7] shows the saving and lists both listing titles", () => {
    render(
      <BundleCard
        bundle={bundle}
        shopListings={[l1, l2] as never}
        shopId="s1"
      />,
    );
    expect(screen.getByText(/Tiết kiệm 30\.000 VND/)).toBeInTheDocument();
    expect(screen.getByText(/Áo thun/)).toBeInTheDocument();
    expect(screen.getByText(/Quần short/)).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Thêm combo vào giỏ" }),
    ).toBeInTheDocument();
  });

  it("[C2] shows unavailable with no add button when a listing is missing from the shop listings", () => {
    render(
      <BundleCard bundle={bundle} shopListings={[l1] as never} shopId="s1" />,
    );
    expect(screen.getByText("Combo tạm không khả dụng")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Thêm combo vào giỏ" }),
    ).toBeNull();
    expect(screen.queryByText(/Tiết kiệm/)).toBeNull();
  });
});
