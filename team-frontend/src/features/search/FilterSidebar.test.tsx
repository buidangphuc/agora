import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
  useSearchParams: () => new URLSearchParams(),
  redirect: vi.fn(),
  notFound: vi.fn(),
}));

import { FilterSidebar } from "./FilterSidebar";
import { priceRangeError } from "./PriceRangeForm";

const facets = {
  categories: [{ key: "c1", count: 5 }],
  priceRanges: [
    { key: "0-100000", count: 2 },
    { key: "100000-500000", count: 3 },
    { key: "1000000+", count: 1 },
  ],
  ratings: [{ key: "4", count: 4 }],
  sellers: [{ key: "s1", count: 1 }],
};
const categories = [
  {
    id: "c1",
    name: "Điện thoại",
    iconUrl: "",
    slug: "",
    parentId: "",
    displayOrder: 0,
  },
];

function setup(
  props: Partial<React.ComponentProps<typeof FilterSidebar>> = {},
) {
  return render(
    <FilterSidebar
      facets={facets}
      categories={categories}
      currentQuery="ao"
      sellerNames={{ s1: "Shop Một" }}
      {...props}
    />,
  );
}

beforeEach(() => push.mockClear());

describe("FilterSidebar", () => {
  it("renders buckets as links with counts and keeps q and sort", () => {
    const { container } = setup({ currentSort: "newest" });
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const bucket = inline.querySelector(
      '[data-testid="facet-bucket"][data-key="100000-500000"]',
    ) as HTMLAnchorElement;
    expect(bucket).toHaveAttribute(
      "href",
      "/search?q=ao&minPrice=100000&maxPrice=500000&sort=newest",
    );
    expect(bucket.textContent).toContain("(3)");
    expect(bucket).toHaveAttribute("data-active", "false");
    expect(within(inline).getByText("Shop Một")).toBeInTheDocument();
  });

  it("marks the active bucket and links back to the unfiltered URL", () => {
    const { container } = setup({
      currentMinPrice: 100000,
      currentMaxPrice: 500000,
    });
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const bucket = inline.querySelector(
      '[data-key="100000-500000"]',
    ) as HTMLAnchorElement;
    expect(bucket).toHaveAttribute("data-active", "true");
    expect(bucket).toHaveAttribute("aria-current", "true");
    expect(bucket).toHaveAttribute("href", "/search?q=ao");
  });

  it("labels the open-ended top bucket and links it as a minimum price", () => {
    const { container } = setup({ currentMinPrice: 1000000 });
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const bucket = inline.querySelector(
      '[data-key="1000000+"]',
    ) as HTMLAnchorElement;
    expect(bucket.textContent).toContain("Trên 1.000.000₫");
    expect(bucket.textContent).not.toContain("NaN");
    expect(bucket).toHaveAttribute("data-active", "true");
    const other = inline.querySelector(
      '[data-key="100000-500000"]',
    ) as HTMLAnchorElement;
    expect(other).toHaveAttribute("data-active", "false");
  });

  it("links the open-ended bucket with minPrice only", () => {
    const { container } = setup();
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const bucket = inline.querySelector(
      '[data-key="1000000+"]',
    ) as HTMLAnchorElement;
    expect(bucket).toHaveAttribute("href", "/search?q=ao&minPrice=1000000");
  });

  it("shows a count badge on the mobile trigger", () => {
    setup({ currentCategory: "c1", currentRating: "4" });
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    expect(within(trigger).getByText("2")).toBeInTheDocument();
  });

  it("opens a drawer with the filters, and Escape returns focus to the trigger", () => {
    setup();
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Danh mục")).toBeInTheDocument();
    expect(within(dialog).getByText("Khoảng giá")).toBeInTheDocument();
    expect(within(dialog).getByText("Đánh giá")).toBeInTheDocument();
    expect(within(dialog).getByText("Nơi bán")).toBeInTheDocument();
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(
      within(dialog).getByRole("button", { name: "Áp dụng" }),
    ).toHaveAttribute("form", "drawer-price-form");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  it("rejects min greater than max without navigating", () => {
    const { container } = setup();
    const form = container.querySelector(
      "#inline-price-form",
    ) as HTMLFormElement;
    const [min, max] = form.querySelectorAll<HTMLInputElement>(
      'input[type="number"]',
    );
    fireEvent.change(min as HTMLInputElement, { target: { value: "500000" } });
    fireEvent.change(max as HTMLInputElement, { target: { value: "100000" } });
    fireEvent.submit(form);
    expect(push).not.toHaveBeenCalled();
    expect(
      within(form).getByText(/Giá tối đa phải lớn hơn/),
    ).toBeInTheDocument();
    expect(max).toHaveAttribute("aria-invalid", "true");
    expect(
      within(form).getByRole("button", { name: "Áp dụng" }),
    ).not.toBeDisabled();
  });

  it("navigates with a valid range and keeps the other params", () => {
    const { container } = setup({ currentSort: "newest" });
    const form = container.querySelector(
      "#inline-price-form",
    ) as HTMLFormElement;
    const [min, max] = form.querySelectorAll<HTMLInputElement>(
      'input[type="number"]',
    );
    fireEvent.change(min as HTMLInputElement, { target: { value: "1000" } });
    fireEvent.change(max as HTMLInputElement, { target: { value: "5000" } });
    fireEvent.submit(form);
    expect(push).toHaveBeenCalledWith(
      "/search?q=ao&sort=newest&minPrice=1000&maxPrice=5000",
    );
  });

  it("has a no-JS fallback: a GET form with hidden params", () => {
    const { container } = setup();
    const form = container.querySelector(
      "#inline-price-form",
    ) as HTMLFormElement;
    expect(form).toHaveAttribute("method", "get");
    expect(form).toHaveAttribute("action", "/search");
    expect(
      form.querySelector('input[type="hidden"][name="q"]'),
    ).toHaveAttribute("value", "ao");
  });
});

describe("priceRangeError", () => {
  it("validates ranges", () => {
    expect(priceRangeError("", "")).toBeUndefined();
    expect(priceRangeError("100", "")).toBeUndefined();
    expect(priceRangeError("100", "200")).toBeUndefined();
    expect(priceRangeError("200", "100")).toMatch(/lớn hơn/);
    expect(priceRangeError("-1", "")).toBe("Giá không hợp lệ.");
  });
});
