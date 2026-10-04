import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import SellerError from "./error";
import SellerLoading from "./loading";
import SellerNotFound from "./not-found";

describe("seller segment states", () => {
  it("loading renders a busy skeleton", () => {
    render(<SellerLoading />);
    expect(screen.getByTestId("seller-skeleton")).toHaveAttribute(
      "aria-busy",
      "true",
    );
  });

  it("error offers retry (reset) and a link to /seller", () => {
    const reset = vi.fn();
    render(<SellerError error={new Error("x")} reset={reset} />);
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(reset).toHaveBeenCalledTimes(1);
    expect(
      screen.getByRole("link", { name: "Về Kênh người bán" }),
    ).toHaveAttribute("href", "/seller");
  });

  it("not-found renders a 404 Result linking to /seller", () => {
    render(<SellerNotFound />);
    expect(screen.getByText("Không tìm thấy nội dung")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Về Kênh người bán" }),
    ).toHaveAttribute("href", "/seller");
  });
});
