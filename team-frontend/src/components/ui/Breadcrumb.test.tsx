import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Breadcrumb } from "./Breadcrumb";

describe("Breadcrumb", () => {
  const items = [
    { label: "Trang chủ", href: "/" },
    { label: "Điện thoại", href: "/search?category=phones" },
    { label: "iPhone 15" },
  ];

  it("is a labelled navigation list with links for ancestors", () => {
    render(<Breadcrumb items={items} />);
    expect(
      screen.getByRole("navigation", { name: "Breadcrumb" }),
    ).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    const links = screen.getAllByRole("link");
    expect(links.map((l) => l.getAttribute("href"))).toEqual([
      "/",
      "/search?category=phones",
    ]);
  });

  it("marks the last item as the current page and not a link", () => {
    render(<Breadcrumb items={items} />);
    const current = screen.getByText("iPhone 15");
    expect(current).toHaveAttribute("aria-current", "page");
    expect(current.closest("a")).toBeNull();
  });

  it("links use a keyboard-visible focus ring", () => {
    render(<Breadcrumb items={items} />);
    expect(screen.getAllByRole("link")[0].className).toContain(
      "focus-visible:ring-2",
    );
  });
});
