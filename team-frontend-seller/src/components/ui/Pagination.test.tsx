import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Pagination } from "./Pagination";

const hrefFor = (p: number) => `/search?page=${p}`;

describe("Pagination", () => {
  it("renders links to pages 1-5 with page 2 current", () => {
    render(
      <Pagination current={2} total={50} pageSize={10} hrefFor={hrefFor} />,
    );
    for (let p = 1; p <= 5; p++) {
      expect(screen.getByRole("link", { name: `Trang ${p}` })).toHaveAttribute(
        "href",
        hrefFor(p),
      );
    }
    expect(screen.getByRole("link", { name: "Trang 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Trang 1" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("works as plain server-rendered links", () => {
    const html = renderToStaticMarkup(
      <Pagination current={2} total={50} pageSize={10} hrefFor={hrefFor} />,
    );
    expect(html).toContain('href="/search?page=1"');
    expect(html).toContain('aria-current="page"');
    expect(html).not.toContain("<button");
  });

  it("prev/next are links in the middle and aria-disabled at the bounds", () => {
    const { rerender } = render(
      <Pagination current={1} total={50} hrefFor={hrefFor} />,
    );
    expect(screen.queryByRole("link", { name: "Trang trước" })).toBeNull();
    expect(screen.getByLabelText("Trang trước")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("link", { name: "Trang sau" })).toHaveAttribute(
      "href",
      hrefFor(2),
    );
    rerender(<Pagination current={5} total={50} hrefFor={hrefFor} />);
    expect(screen.getByLabelText("Trang sau")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("link", { name: "Trang trước" })).toHaveAttribute(
      "href",
      hrefFor(4),
    );
  });

  it("collapses long ranges with ellipses around the current page", () => {
    render(<Pagination current={10} total={200} hrefFor={hrefFor} />);
    const names = screen
      .getAllByRole("link")
      .map((l) => l.getAttribute("aria-label"));
    expect(names).toEqual([
      "Trang trước",
      "Trang 1",
      "Trang 9",
      "Trang 10",
      "Trang 11",
      "Trang 20",
      "Trang sau",
    ]);
  });

  it("renders nothing for a single page and clamps an out-of-range current", () => {
    const { container, rerender } = render(
      <Pagination current={1} total={5} hrefFor={hrefFor} />,
    );
    expect(container).toBeEmptyDOMElement();
    rerender(<Pagination current={99} total={30} hrefFor={hrefFor} />);
    expect(screen.getByRole("link", { name: "Trang 3" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });
});
