import { fireEvent, render, screen } from "@testing-library/react";
import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const refresh = vi.fn();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh }),
}));

import CheckoutError from "./(checkout)/checkout/error";
import AccountError from "./(shop)/account/error";
import OrdersError from "./(shop)/account/orders/error";
import CartError from "./(shop)/cart/error";
import FavoritesError from "./(shop)/favorites/error";
import ListingError from "./(shop)/listing/error";
import NotificationsError from "./(shop)/notifications/error";
import SellerError from "./(shop)/seller/error";
import RootError from "./error";

const boundaries = {
  root: RootError,
  seller: SellerError,
  orders: OrdersError,
  checkout: CheckoutError,
  cart: CartError,
  listing: ListingError,
  account: AccountError,
  favorites: FavoritesError,
  notifications: NotificationsError,
};

describe("route error boundaries: retry re-fetches server data", () => {
  beforeEach(() => refresh.mockClear());

  for (const [name, Boundary] of Object.entries(boundaries)) {
    it(`${name}: "Thử lại" calls router.refresh() and reset()`, () => {
      const reset = vi.fn();
      render(<Boundary error={new Error("boom")} reset={reset} />);
      fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
      expect(refresh).toHaveBeenCalledTimes(1);
      expect(reset).toHaveBeenCalledTimes(1);
    });
  }
});
