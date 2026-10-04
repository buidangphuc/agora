import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { ReviewRatingFilter } from "./ReviewRatingFilter";

const replace = vi.fn();
let search = "";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace }),
  usePathname: () => "/listing/L",
  useSearchParams: () => new URLSearchParams(search),
}));

const breakdown = { star1: 1, star2: 0, star3: 2, star4: 3, star5: 4 };

beforeEach(() => {
  replace.mockClear();
  search = "";
});

describe("ReviewRatingFilter", () => {
  it("writes ?rating= on a star click without a scroll jump, keeping the #reviews hash", async () => {
    const user = setupUser();
    render(<ReviewRatingFilter total={10} breakdown={breakdown} current={0} />);
    await user.click(screen.getByRole("button", { name: "4 Sao (3)" }));
    expect(replace).toHaveBeenCalledWith("/listing/L?rating=4#reviews", {
      scroll: false,
    });
  });

  it("keeps other params, drops the page, and clears rating on Tất cả", async () => {
    const user = setupUser();
    search = "variant=v2&rating=4&rpage=3";
    render(<ReviewRatingFilter total={10} breakdown={breakdown} current={4} />);
    await user.click(screen.getByRole("button", { name: "5 Sao (4)" }));
    expect(replace).toHaveBeenLastCalledWith(
      "/listing/L?variant=v2&rating=5#reviews",
      { scroll: false },
    );
    await user.click(screen.getByRole("button", { name: "Tất cả (10)" }));
    expect(replace).toHaveBeenLastCalledWith("/listing/L?variant=v2#reviews", {
      scroll: false,
    });
  });

  it("marks the active filter and ignores a click on it", async () => {
    const user = setupUser();
    render(<ReviewRatingFilter total={10} breakdown={breakdown} current={4} />);
    expect(screen.getByRole("button", { name: "4 Sao (3)" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Tất cả (10)" })).toHaveAttribute(
      "aria-pressed",
      "false",
    );
    await user.click(screen.getByRole("button", { name: "4 Sao (3)" }));
    expect(replace).not.toHaveBeenCalled();
  });

  it("offers All and 5..1 stars with counts from the breakdown", () => {
    render(<ReviewRatingFilter total={10} breakdown={breakdown} current={0} />);
    const names = screen
      .getAllByTestId("review-filter")
      .map((b) => b.textContent);
    expect(names).toEqual([
      "Tất cả (10)",
      "5 Sao (4)",
      "4 Sao (3)",
      "3 Sao (2)",
      "2 Sao (0)",
      "1 Sao (1)",
    ]);
  });
});
