import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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
  ratings: [
    { key: "rating_floor_4", count: 4 },
    { key: "rating_floor_3", count: 7 },
  ],
  sellers: [{ key: "s1", count: 1 }],
  tags: [],
  skus: [],
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
  it("The search page shows the rating filter: Từ 4 sao and Từ 3 sao with their counts", () => {
    const { container } = setup();
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const group = within(inline).getByTestId("facet-ratings");
    const buckets = within(group).getAllByTestId("facet-bucket");
    expect(buckets).toHaveLength(2);
    const four = buckets.find((b) => b.textContent?.includes("Từ 4 sao"));
    const three = buckets.find((b) => b.textContent?.includes("Từ 3 sao"));
    expect(four?.textContent).toContain("(4)");
    expect(three?.textContent).toContain("(7)");
    expect(four).toHaveAttribute("data-active", "false");
    expect(three).toHaveAttribute("data-active", "false");
  });

  it("Choosing a rating filters the results by URL: links carry minRating, keep q and sort, drop page and never carry rating=", () => {
    const { container } = setup({ currentSort: "newest" });
    const group = within(
      container.querySelector(".hidden.lg\\:block") as HTMLElement,
    ).getByTestId("facet-ratings");
    const four = within(group)
      .getByText(/Từ 4 sao/)
      .closest("a") as HTMLAnchorElement;
    const three = within(group)
      .getByText(/Từ 3 sao/)
      .closest("a") as HTMLAnchorElement;
    const p4 = new URL(`http://x${four.getAttribute("href")}`).searchParams;
    expect(p4.get("minRating")).toBe("4");
    expect(p4.get("q")).toBe("ao");
    expect(p4.get("sort")).toBe("newest");
    expect(p4.get("page")).toBeNull();
    expect(three.getAttribute("href")).toContain("minRating=3");
    expect(container.innerHTML).not.toMatch(/[?&]rating=/);
  });

  it("The rating group keeps both options while one is selected, marking the selected one and linking it back to unfiltered", () => {
    const { container } = setup({ currentMinRating: 4 });
    const group = within(
      container.querySelector(".hidden.lg\\:block") as HTMLElement,
    ).getByTestId("facet-ratings");
    const four = within(group)
      .getByText(/Từ 4 sao/)
      .closest("a") as HTMLAnchorElement;
    const three = within(group)
      .getByText(/Từ 3 sao/)
      .closest("a") as HTMLAnchorElement;
    expect(four).toHaveAttribute("data-active", "true");
    expect(four).toHaveAttribute("aria-current", "true");
    expect(four).toHaveAttribute("href", "/search?q=ao");
    expect(three).toHaveAttribute("data-active", "false");
    expect(three.textContent).toContain("(7)");
    expect(three.getAttribute("href")).toContain("minRating=3");
  });

  it("keeps minRating in the links of other filter groups", () => {
    const { container } = setup({ currentMinRating: 3 });
    const inline = container.querySelector(".hidden.lg\\:block") as HTMLElement;
    const bucket = inline.querySelector(
      '[data-key="100000-500000"]',
    ) as HTMLAnchorElement;
    expect(bucket.getAttribute("href")).toContain("minRating=3");
  });

  it("shows no rating group when facets.ratings is empty", () => {
    const { container } = setup({ facets: { ...facets, ratings: [] } });
    expect(container.querySelector('[data-testid="facet-ratings"]')).toBeNull();
  });

  it("The rating filter is offered in the mobile drawer", () => {
    setup({ currentMinRating: 4 });
    fireEvent.click(screen.getByRole("button", { name: /Bộ lọc/ }));
    const dialog = screen.getByRole("dialog");
    const group = within(dialog).getByTestId("facet-ratings");
    const three = within(group)
      .getByText(/Từ 3 sao/)
      .closest("a") as HTMLAnchorElement;
    expect(three.textContent).toContain("(7)");
    expect(three.getAttribute("href")).toContain("minRating=3");
    expect(
      within(group)
        .getByText(/Từ 4 sao/)
        .closest("a"),
    ).toHaveAttribute("data-active", "true");
  });

  it("counts the rating once on the mobile trigger", () => {
    setup({ currentMinRating: 4 });
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    expect(within(trigger).getByText("1")).toBeInTheDocument();
  });

  it("counts the rating with the other filters on the mobile trigger", () => {
    setup({ currentCategory: "c1", currentMinRating: 3 });
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    expect(within(trigger).getByText("2")).toBeInTheDocument();
  });

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
    setup({ currentCategory: "c1", currentSeller: "s1" });
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    expect(within(trigger).getByText("2")).toBeInTheDocument();
  });

  it("opens a drawer with the filters, and Escape returns focus to the trigger", async () => {
    setup();
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByText("Danh mục")).toBeInTheDocument();
    expect(within(dialog).getByText("Khoảng giá")).toBeInTheDocument();
    expect(within(dialog).getByTestId("facet-ratings")).toBeInTheDocument();
    expect(within(dialog).getByText("Nơi bán")).toBeInTheDocument();
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(
      within(dialog).getByRole("button", { name: "Áp dụng" }),
    ).toHaveAttribute("form", "drawer-price-form");
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(trigger));
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

describe("FilterSidebar dynamic facets", () => {
  const dyn = {
    ...facets,
    tags: [
      {
        group: "connectivity",
        buckets: [
          { key: "bluetooth-5-3", count: 4 },
          { key: "wifi-6", count: 1 },
        ],
      },
      { group: "color", buckets: [{ key: "den", count: 9 }] },
    ],
    skus: [
      {
        group: "color",
        buckets: [
          { key: "xanh-navy", count: 2 },
          { key: "den", count: 3 },
        ],
      },
      { group: "capacity", buckets: [{ key: "512gb", count: 2 }] },
    ],
  };
  const inlineOf = (c: HTMLElement) =>
    c.querySelector(".hidden.lg\\:block") as HTMLElement;

  it("renders one group per dynamic facet with counts, variant groups win over a same-named tag group", () => {
    const { container } = setup({ facets: dyn });
    const inline = inlineOf(container);
    expect(
      within(inline).getByTestId("facet-tag-connectivity"),
    ).toBeInTheDocument();
    expect(within(inline).getByTestId("facet-sku-color")).toBeInTheDocument();
    expect(
      within(inline).getByTestId("facet-sku-capacity"),
    ).toBeInTheDocument();
    expect(within(inline).queryByTestId("facet-tag-color")).toBeNull();
    expect(within(inline).getByText("Kết nối")).toBeInTheDocument();
    expect(within(inline).getByText("Dung lượng")).toBeInTheDocument();
    const navy = inline.querySelector(
      '[data-testid="facet-sku-color"] [data-key="xanh-navy"]',
    ) as HTMLAnchorElement;
    expect(navy.textContent).toContain("(2)");
    expect(navy).toHaveAttribute("href", "/search?q=ao&sku.color=xanh-navy");
  });

  it("toggles a value in the URL: a second value is added, a selected one removed", () => {
    const { container } = setup({
      facets: dyn,
      currentAttrs: {
        "sku.color": ["xanh-navy"],
        "tag.connectivity": ["wifi-6"],
      },
    });
    const inline = inlineOf(container);
    const den = inline.querySelector(
      '[data-testid="facet-sku-color"] [data-key="den"]',
    ) as HTMLAnchorElement;
    expect(den).toHaveAttribute(
      "href",
      "/search?q=ao&sku.color=xanh-navy%2Cden&tag.connectivity=wifi-6",
    );
    const navy = inline.querySelector(
      '[data-testid="facet-sku-color"] [data-key="xanh-navy"]',
    ) as HTMLAnchorElement;
    expect(navy).toHaveAttribute("data-active", "true");
    expect(navy).toHaveAttribute(
      "href",
      "/search?q=ao&tag.connectivity=wifi-6",
    );
  });

  it("counts a selected dynamic group on the mobile trigger and renders in the drawer", () => {
    setup({ facets: dyn, currentAttrs: { "sku.color": ["den"] } });
    const trigger = screen.getByRole("button", { name: /Bộ lọc/ });
    expect(within(trigger).getByText("1")).toBeInTheDocument();
    fireEvent.click(trigger);
    expect(
      within(screen.getByRole("dialog")).getByTestId("facet-sku-capacity"),
    ).toBeInTheDocument();
  });

  it("shows no dynamic group when the response carries none", () => {
    const { container } = setup();
    expect(
      inlineOf(container).querySelector(
        '[data-testid^="facet-tag-"],[data-testid^="facet-sku-"]',
      ),
    ).toBeNull();
  });
});
