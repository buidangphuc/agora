import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import CheckoutLayout from "./layout";

vi.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams("step=payment&addr=a1&pay=2"),
}));

describe("checkout shell", () => {
  it("renders the minimal header, Stepper and secure footer without search or bottom nav", () => {
    const { container } = render(
      <CheckoutLayout>
        <p>child</p>
      </CheckoutLayout>,
    );
    expect(screen.getByRole("link", { name: "Marketplace" })).toHaveAttribute(
      "href",
      "/",
    );
    expect(
      screen.getByRole("navigation", { name: "Progress" }),
    ).toBeInTheDocument();
    for (const t of ["Địa chỉ", "Vận chuyển", "Thanh toán", "Xác nhận"]) {
      expect(screen.getAllByText(t).length).toBeGreaterThan(0);
    }
    expect(
      screen.getAllByText(/Thanh toán an toàn/).length,
    ).toBeGreaterThanOrEqual(2);
    expect(screen.getByText("child")).toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).toBeNull();
    expect(
      container.querySelector("nav[aria-label='Điều hướng dưới']"),
    ).toBeNull();
  });

  it("is its own shell: no injected CSS and no consumer chrome", () => {
    const { container } = render(
      <CheckoutLayout>
        <p>child</p>
      </CheckoutLayout>,
    );
    expect(container.querySelector("style")).toBeNull();
    expect(container.querySelector("[data-checkout-shell]")).not.toBeNull();
    expect(container.querySelectorAll("header")).toHaveLength(1);
    expect(container.querySelectorAll("footer")).toHaveLength(1);
  });

  it("completed steps are links that keep the selections; the current step is not", () => {
    render(
      <CheckoutLayout>
        <p>child</p>
      </CheckoutLayout>,
    );
    expect(screen.getByRole("link", { name: "Địa chỉ" })).toHaveAttribute(
      "href",
      "/checkout?step=address&addr=a1&pay=2",
    );
    expect(screen.getByRole("link", { name: "Vận chuyển" })).toHaveAttribute(
      "href",
      "/checkout?step=shipping&addr=a1&pay=2",
    );
    expect(screen.queryByRole("link", { name: "Thanh toán" })).toBeNull();
    expect(screen.getByText(/Bước 3\/4/)).toBeInTheDocument();
  });
});
