import { fireEvent, render, screen, within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach, describe, expect, expectTypeOf, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

const trackEcommerce = vi.hoisted(() => vi.fn());
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => <button type="button">fav</button>,
}));

import { ListingCard } from "./ListingCard";
import { ListingGrid } from "./ListingGrid";
import { ListingCardSkeleton, ListingGridSkeleton } from "./ListingSkeleton";

function listing(over: Partial<ViewListing> = {}): ViewListing {
  return {
    id: "l1",
    title: "Áo thun cotton",
    description: "",
    price: 1250000,
    currency: "VND",
    status: "published",
    sellerId: "s1",
    imageKeys: [],
    imageUrl: "https://img.test/a.jpg",
    categoryId: "c1",
    stock: 5,
    variants: [],
    ...over,
  };
}

function many(n: number): ViewListing[] {
  return Array.from({ length: n }, (_, i) =>
    listing({ id: `l${i + 1}`, imageUrl: `https://img.test/${i + 1}.jpg` }),
  );
}

beforeEach(() => trackEcommerce.mockClear());

describe("ListingCard", () => {
  it("reserves a 1:1 image box", () => {
    const { container } = render(<ListingCard listing={listing()} />);
    expect(container.querySelector(".aspect-square")).not.toBeNull();
    expect(screen.getByRole("img", { name: "Áo thun cotton" })).toHaveAttribute(
      "src",
      "https://img.test/a.jpg",
    );
  });

  it("shows a placeholder inside the same 1:1 box without an image", () => {
    const { container } = render(
      <ListingCard listing={listing({ imageUrl: undefined })} />,
    );
    const box = container.querySelector(".aspect-square");
    expect(box).not.toBeNull();
    expect(
      within(box as HTMLElement).getByRole("img", { name: "Không có ảnh" }),
    ).toBeInTheDocument();
  });

  it("loads the first 6 images eagerly and the rest lazily", () => {
    render(<ListingGrid listings={many(24)} />);
    const imgs = screen.getAllByRole("img");
    expect(imgs).toHaveLength(24);
    for (const img of imgs.slice(0, 6)) {
      expect(img).not.toHaveAttribute("loading", "lazy");
    }
    for (const img of imgs.slice(6)) {
      expect(img).toHaveAttribute("loading", "lazy");
    }
  });

  it("renders no rating, no sold count and no fabricated promo without reviews", () => {
    const { container } = render(<ListingCard listing={listing()} />);
    expect(screen.queryByLabelText(/trên 5 sao/)).toBeNull();
    expect(container.textContent).not.toMatch(/Đã bán|5\.0|★|Giảm|Yêu thích/);
    expect(container.querySelector(".line-through")).toBeNull();
  });

  it("renders a rating only for a real review aggregate", () => {
    const { rerender } = render(
      <ListingCard listing={listing()} review={{ average: 4.5, count: 12 }} />,
    );
    expect(screen.getByLabelText("4.5 trên 5 sao")).toBeInTheDocument();
    expect(screen.getByText("(12)")).toBeInTheDocument();
    rerender(
      <ListingCard listing={listing()} review={{ average: 5, count: 0 }} />,
    );
    expect(screen.queryByLabelText(/trên 5 sao/)).toBeNull();
  });

  it("shows only the real price and never a MALL badge", () => {
    for (const l of [
      listing({ price: 6000000 }),
      listing({ title: "Điện thoại Apple chính hãng" }),
    ]) {
      const { container, unmount } = render(<ListingCard listing={l} />);
      expect(container.textContent).not.toMatch(/MALL/i);
      expect(container.querySelector(".line-through")).toBeNull();
      expect(container.textContent).not.toMatch(/-\d+%/);
      expect(container.textContent?.match(/₫/g)).toHaveLength(1);
      unmount();
    }
  });

  it("shows Freeship only when told so", () => {
    const { rerender } = render(<ListingCard listing={listing()} />);
    expect(screen.queryByText("Freeship")).toBeNull();
    rerender(<ListingCard listing={listing()} freeShip />);
    expect(screen.getByText("Freeship")).toBeInTheDocument();
  });

  it("keeps the tracking payloads unchanged", () => {
    render(
      <ListingCard
        listing={listing({ id: "abc" })}
        placementId="home_recommendations"
        impressionId="i1"
        modelVersion="m1"
        position={3}
      />,
    );
    expect(trackEcommerce).toHaveBeenCalledTimes(1);
    expect(trackEcommerce).toHaveBeenCalledWith("view_item_list", {
      query: undefined,
      properties: undefined,
      items: [
        {
          itemId: "abc",
          placementId: "home_recommendations",
          impressionId: "i1",
          modelVersion: "m1",
          index: 3,
          price: undefined,
          itemCategory: undefined,
          itemListId: "home_recommendations",
        },
      ],
    });
    const image = screen.getByRole("img", { name: "Áo thun cotton" });
    const imageLink = image.closest("a") as HTMLAnchorElement;
    expect(imageLink).toHaveAttribute("href", "/listing/abc");
    imageLink.addEventListener("click", (e) => e.preventDefault());
    fireEvent.click(imageLink);
    expect(trackEcommerce).toHaveBeenLastCalledWith("select_item", {
      items: [
        {
          itemId: "abc",
          index: 3,
          placementId: "home_recommendations",
          impressionId: "i1",
          modelVersion: "m1",
          itemListId: "home_recommendations",
          price: undefined,
        },
      ],
    });
  });
});

describe("ListingGrid", () => {
  it("uses 2/3/4/6 columns and passes position = index + 1", () => {
    const { container } = render(
      <ListingGrid
        listings={many(3)}
        placementId="p"
        impressionId="i"
        modelVersion="m"
      />,
    );
    const grid = container.firstElementChild as HTMLElement;
    for (const c of [
      "grid-cols-2",
      "sm:grid-cols-3",
      "md:grid-cols-4",
      "lg:grid-cols-6",
    ]) {
      expect(grid).toHaveClass(c);
    }
    expect(grid.className).not.toMatch(/text-\[/);
    expect(trackEcommerce.mock.calls.map((c) => c[1].items[0])).toEqual([
      expect.objectContaining({ itemId: "l1", index: 1, placementId: "p" }),
      expect.objectContaining({ itemId: "l2", index: 2, impressionId: "i" }),
      expect.objectContaining({ itemId: "l3", index: 3, modelVersion: "m" }),
    ]);
  });

  it("renders an Empty with the message and a way out", () => {
    render(<ListingGrid listings={[]} empty="Không có gì" />);
    expect(screen.getByText("Không có gì")).toBeInTheDocument();
    expect(screen.getByText(/thay đổi bộ lọc/)).toBeInTheDocument();
  });

  it("keeps its public props additive", () => {
    type OldGridProps = {
      listings: ViewListing[];
      empty?: string;
      placementId?: string;
      impressionId?: string;
      modelVersion?: string;
    };
    type OldCardProps = {
      listing: ViewListing;
      placementId?: string;
      impressionId?: string;
      modelVersion?: string;
      position?: number;
    };
    expectTypeOf<OldGridProps>().toMatchTypeOf<
      ComponentProps<typeof ListingGrid>
    >();
    expectTypeOf<OldCardProps>().toMatchTypeOf<
      ComponentProps<typeof ListingCard>
    >();
  });
});

describe("ListingGridSkeleton", () => {
  it("shares the grid of ListingGrid", () => {
    const { container } = render(<ListingGridSkeleton count={24} />);
    const grid = container.firstElementChild as HTMLElement;
    expect(grid).toHaveClass("grid-cols-2", "lg:grid-cols-6");
    expect(grid.children).toHaveLength(24);
  });

  it("has a 1:1 image box like the card", () => {
    const { container } = render(<ListingCardSkeleton />);
    expect(container.querySelector(".aspect-square")).not.toBeNull();
  });
});
