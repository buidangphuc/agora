import { render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import ListingError from "../error";
import ListingNotFound from "../not-found";
import Loading from "./loading";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

describe("loading.tsx", () => {
  it("renders Skeletons with the footprints of the final anatomy (gallery 1:1)", () => {
    const { container } = render(<Loading />);
    const root = screen.getByTestId("pdp-skeleton");
    expect(root).toHaveAttribute("aria-busy", "true");
    // The gallery stage skeleton reserves a square box.
    expect(container.querySelector(".aspect-square")).not.toBeNull();
    // breadcrumb + header card + shop card + anchor nav + a body card.
    expect(
      container.querySelectorAll("[data-variant]").length,
    ).toBeGreaterThanOrEqual(8);
    // Nothing interactive, nothing invented.
    expect(within(root).queryAllByRole("button")).toHaveLength(0);
    expect(within(root).queryAllByRole("link")).toHaveLength(0);
  });
});

describe("not-found.tsx", () => {
  it("renders Result 404 with the product title and both recovery actions", () => {
    render(<ListingNotFound />);
    expect(
      screen.getByRole("img", { name: "Không tìm thấy" }),
    ).toHaveTextContent("404");
    expect(
      screen.getByRole("heading", { name: "Không tìm thấy sản phẩm" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Về trang chủ" })).toHaveAttribute(
      "href",
      "/",
    );
    expect(
      screen.getByRole("link", { name: "Tìm sản phẩm khác" }),
    ).toHaveAttribute("href", "/search");
  });
});

describe("error.tsx", () => {
  it("renders an error Alert whose Thử lại action calls reset()", async () => {
    const user = setupUser();
    const reset = vi.fn();
    render(<ListingError error={new Error("boom")} reset={reset} />);
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Không thể tải sản phẩm");
    // The raw error message is not leaked to the buyer.
    expect(alert).not.toHaveTextContent("boom");
    await user.click(within(alert).getByRole("button", { name: "Thử lại" }));
    expect(reset).toHaveBeenCalledTimes(1);
  });
});
