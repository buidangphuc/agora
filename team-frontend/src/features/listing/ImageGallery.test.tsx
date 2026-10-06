import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { PurchaseProvider } from "@/features/cart/PurchaseContext";
import { setupUser } from "@/test/user";
import { ImageGallery } from "./ImageGallery";

import { vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));
vi.mock("@/features/cart/actions", () => ({ addToCartAction: vi.fn() }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));

const IMAGES = [
  "http://x/1.png",
  "http://x/2.png",
  "http://x/3.png",
  "http://x/4.png",
];

function stageImg(): HTMLImageElement {
  const stage = screen.getByTestId("gallery-stage");
  return stage.querySelector("img") as HTMLImageElement;
}

describe("ImageGallery", () => {
  it("swaps the stage image on a thumbnail click without changing the stage box", async () => {
    const user = setupUser();
    render(<ImageGallery images={IMAGES} alt="Áo" />);
    const stage = screen.getByTestId("gallery-stage");
    const classBefore = stage.className;
    expect(stage.querySelector(".aspect-square")).not.toBeNull();
    expect(stageImg()).toHaveAttribute("src", IMAGES[0]);

    await user.click(screen.getByRole("button", { name: "Ảnh 3" }));

    expect(stageImg()).toHaveAttribute("src", IMAGES[2]);
    expect(stage.className).toBe(classBefore);
    expect(stage.querySelector(".aspect-square")).not.toBeNull();
    expect(screen.getByRole("button", { name: "Ảnh 3" })).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(screen.getByRole("button", { name: "Ảnh 1" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("moves the selection with the arrow keys and selects with Enter/Space", async () => {
    const user = setupUser();
    render(<ImageGallery images={IMAGES} alt="Áo" />);
    const first = screen.getByRole("button", { name: "Ảnh 1" });
    first.focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Ảnh 2" })).toHaveAttribute(
      "aria-current",
      "true",
    );
    expect(screen.getByRole("button", { name: "Ảnh 2" })).toHaveFocus();
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(screen.getByRole("button", { name: "Ảnh 4" })).toHaveAttribute(
      "aria-current",
      "true",
    );
    await user.keyboard("{ArrowLeft}");
    await user.keyboard(" ");
    expect(stageImg()).toHaveAttribute("src", IMAGES[2]);
    await user.keyboard("{ArrowRight}{Enter}");
    expect(stageImg()).toHaveAttribute("src", IMAGES[3]);
  });

  it("renders the Image fallback in the same 1:1 box when there are no images", () => {
    render(<ImageGallery images={[]} alt="Áo" />);
    const stage = screen.getByTestId("gallery-stage");
    expect(stage.querySelector(".aspect-square")).not.toBeNull();
    expect(screen.queryByRole("button", { name: /Ảnh/ })).toBeNull();
    // No external stock photo is requested.
    for (const img of Array.from(document.querySelectorAll("img"))) {
      expect(img.getAttribute("src") ?? "").not.toContain("unsplash");
    }
    expect(
      screen.getByRole("img", { name: "Không có ảnh" }),
    ).toBeInTheDocument();
  });

  it("loads the stage eagerly and the thumbnails lazily", () => {
    render(<ImageGallery images={IMAGES} alt="Áo" />);
    expect(stageImg()).toHaveAttribute("loading", "eager");
    const thumbs = screen
      .getAllByRole("button", { name: /^Ảnh \d$/ })
      .map((b) => b.querySelector("img"));
    expect(thumbs).toHaveLength(4);
    for (const img of thumbs) expect(img).toHaveAttribute("loading", "lazy");
  });

  it("shows the selected variant's image and lets a thumbnail override it", async () => {
    const user = setupUser();
    const listing = {
      id: "l1",
      title: "Áo",
      categoryId: "c",
      price: 100,
      stock: 5,
      currency: "VND",
      variants: [
        {
          id: "a",
          listingId: "l1",
          name: "A",
          sku: "",
          price: 0,
          stock: 5,
          imageUrl: "",
        },
        {
          id: "b",
          listingId: "l1",
          name: "B",
          sku: "",
          price: 0,
          stock: 5,
          imageUrl: IMAGES[2],
        },
      ],
    };
    render(
      <PurchaseProvider listing={listing} initialVariantId="b">
        <ImageGallery images={IMAGES} alt="Áo" />
      </PurchaseProvider>,
    );
    expect(stageImg()).toHaveAttribute("src", IMAGES[2]);
    expect(screen.getByRole("button", { name: "Ảnh 3" })).toHaveAttribute(
      "aria-current",
      "true",
    );
    await user.click(screen.getByRole("button", { name: "Ảnh 1" }));
    expect(stageImg()).toHaveAttribute("src", IMAGES[0]);
  });
});
