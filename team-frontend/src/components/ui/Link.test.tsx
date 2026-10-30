import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Link } from "./Link";

describe("Link facade", () => {
  it("keeps href, accessible name and test id", () => {
    render(
      <Link href="/cart" data-testid="go-cart">
        Giỏ hàng
      </Link>,
    );
    const link = screen.getByRole("link", { name: "Giỏ hàng" });
    expect(link).toHaveAttribute("href", "/cart");
    expect(link).toHaveAttribute("data-testid", "go-cart");
  });

  it("maps outline and white to the platform secondary button look", () => {
    render(
      <>
        <Link href="/a" variant="outline">
          A
        </Link>
        <Link href="/b" variant="secondary">
          B
        </Link>
      </>,
    );
    expect(screen.getByRole("link", { name: "A" }).className).toBe(
      screen.getByRole("link", { name: "B" }).className,
    );
  });
});
