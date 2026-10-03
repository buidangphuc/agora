import { fireEvent, render, screen } from "@testing-library/react";
import React from "react";
import { describe, expect, it, vi } from "vitest";

import ErrorPage from "./error";
import Loading from "./loading";
import NotFound from "./not-found";

describe("app exception and loading shells", () => {
  it("not-found shows a 404 result with a link home", () => {
    render(<NotFound />);
    expect(screen.getByText("404")).toBeInTheDocument();
    expect(screen.getByText("Không tìm thấy trang")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Về trang chủ" })).toHaveAttribute(
      "href",
      "/",
    );
  });

  it("error shows a recoverable result whose retry calls reset()", () => {
    const reset = vi.fn();
    render(<ErrorPage error={new Error("boom")} reset={reset} />);
    expect(screen.getByText("Đã có lỗi xảy ra")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("loading renders a skeleton that reserves the page layout", () => {
    render(<Loading />);
    const shell = screen.getByTestId("page-skeleton");
    expect(shell).toHaveAttribute("aria-busy", "true");
    expect(shell.querySelectorAll(".animate-pulse").length).toBeGreaterThan(5);
  });
});
