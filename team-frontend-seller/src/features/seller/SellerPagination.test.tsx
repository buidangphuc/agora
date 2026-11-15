import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { SellerPagination } from "./SellerPagination";

describe("SellerPagination cursor mode", () => {
  it("Next page uses the returned cursor: page 1 links Trang sau to the next cursor", () => {
    render(
      <SellerPagination
        mode="cursor"
        current={1}
        firstHref="/seller/orders"
        nextHref="/seller/orders?cursor=c20&page=2"
      />,
    );
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(
      within(nav).getByRole("link", { name: "Trang sau" }),
    ).toHaveAttribute("href", "/seller/orders?cursor=c20&page=2");
    expect(within(nav).getByText("Trang 1")).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(nav).queryByRole("link", { name: "Trang đầu" })).toBeNull();
  });

  it("Next page uses the returned cursor: page 2 shows Trang đầu without cursor and Trang 2", () => {
    render(
      <SellerPagination
        mode="cursor"
        current={2}
        firstHref="/seller/orders"
        nextHref="/seller/orders?cursor=c40&page=3"
      />,
    );
    const nav = screen.getByRole("navigation", { name: "Phân trang" });
    expect(
      within(nav).getByRole("link", { name: "Trang đầu" }),
    ).toHaveAttribute("href", "/seller/orders");
    expect(within(nav).getByText("Trang 2")).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("Last page has no next link: without nextHref Trang sau is not rendered", () => {
    render(
      <SellerPagination
        mode="cursor"
        current={3}
        firstHref="/seller/orders"
        nextHref={null}
      />,
    );
    expect(screen.queryByText("Trang sau")).toBeNull();
    expect(screen.getByRole("link", { name: "Trang đầu" })).toBeInTheDocument();
  });

  it("Last page has no next link: a single page renders no pagination", () => {
    const { container } = render(
      <SellerPagination
        mode="cursor"
        current={1}
        firstHref="/seller/orders"
        nextHref={null}
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
