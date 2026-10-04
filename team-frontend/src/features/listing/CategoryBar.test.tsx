import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ViewCategory } from "@/lib/gateway/listings";
import { CategoryBar } from "./CategoryBar";

const base = { slug: "", parentId: "", displayOrder: 0 };
const cats: ViewCategory[] = [
  { ...base, id: "c1", name: "Điện thoại", iconUrl: "📱" },
  { ...base, id: "c2", name: "Thời trang", iconUrl: "" },
];

describe("CategoryBar", () => {
  it("renders nothing without categories", () => {
    const { container } = render(<CategoryBar categories={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("links the grid variant to /search?category=<id>", () => {
    render(<CategoryBar categories={cats} variant="grid" />);
    expect(screen.getByRole("link", { name: /Điện thoại/ })).toHaveAttribute(
      "href",
      "/search?category=c1",
    );
    expect(screen.queryByRole("link", { name: "Tất cả" })).toBeNull();
  });

  it("marks the selected pill and not 'Tất cả' with aria-current", () => {
    render(<CategoryBar categories={cats} selectedId="c1" variant="pills" />);
    expect(screen.getByRole("link", { name: /Điện thoại/ })).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(screen.getByRole("link", { name: "Tất cả" })).not.toHaveAttribute(
      "aria-current",
    );
    expect(
      screen.getByRole("link", { name: "Thời trang" }),
    ).not.toHaveAttribute("aria-current");
  });

  it("marks 'Tất cả' current when nothing is selected", () => {
    render(<CategoryBar categories={cats} />);
    expect(screen.getByRole("link", { name: "Tất cả" })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("marks the selected grid tile", () => {
    render(<CategoryBar categories={cats} selectedId="c2" variant="grid" />);
    expect(screen.getByRole("link", { name: /Thời trang/ })).toHaveAttribute(
      "aria-current",
      "true",
    );
  });

  it("honours a custom baseUrl", () => {
    render(<CategoryBar categories={cats} baseUrl="/shop/s1" />);
    expect(screen.getByRole("link", { name: /Điện thoại/ })).toHaveAttribute(
      "href",
      "/shop/s1?category=c1",
    );
  });
});
