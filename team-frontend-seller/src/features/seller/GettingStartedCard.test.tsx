import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { GettingStartedCard } from "./GettingStartedCard";
import type { GettingStartedSources } from "./gettingStarted";

const FULL = { displayName: "Shop An", bannerUrl: "https://cdn/b.png" };
const ONE = { ok: true as const, items: [{ id: "x" }] };
const NONE = { ok: true as const, items: [] };

function sources(over: Partial<GettingStartedSources> = {}): GettingStartedSources {
  return {
    storefront: async () => FULL,
    listings: async () => ({ items: [{ id: "l1" }] }),
    vouchers: async () => ONE,
    campaigns: async () => NONE,
    ...over,
  };
}

async function renderCard(src: GettingStartedSources) {
  const ui = await GettingStartedCard({ sources: src });
  return render(<>{ui}</>);
}

describe("GettingStartedCard", () => {
  it("renders nothing when all steps are done", async () => {
    const { container } = await renderCard(sources());
    expect(screen.queryByText("Bắt đầu bán hàng")).toBeNull();
    expect(container.textContent).toBe("");
  });

  it("failed source shows chua ro and card stays", async () => {
    await renderCard(
      sources({
        listings: async () => {
          throw new Error("unavailable");
        },
      }),
    );
    expect(screen.getByText("Bắt đầu bán hàng")).toBeTruthy();
    const step = screen.getByText("Đăng sản phẩm đầu tiên").closest("li") as HTMLElement;
    expect(within(step).getByText(/chưa rõ/)).toBeTruthy();
    expect(within(step).getByRole("link").getAttribute("href")).toBe("/seller/new");
  });

  it("one missing step keeps the card", async () => {
    await renderCard(sources({ storefront: async () => ({ ...FULL, bannerUrl: "" }) }));
    expect(screen.getByText("Bắt đầu bán hàng")).toBeTruthy();
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(4);
    expect(items.map((i) => i.textContent)).toEqual([
      expect.stringContaining("Đặt tên gian hàng"),
      expect.stringContaining("Thêm ảnh bìa gian hàng"),
      expect.stringContaining("Đăng sản phẩm đầu tiên"),
      expect.stringContaining("Tạo voucher hoặc flash sale đầu tiên"),
    ]);
    const banner = screen.getByText("Thêm ảnh bìa gian hàng").closest("li") as HTMLElement;
    expect(within(banner).getByRole("link").getAttribute("href")).toBe("/seller/shop");
    expect(banner.textContent).not.toMatch(/chưa rõ/);
    for (const i of [0, 2, 3]) {
      expect(items[i].getAttribute("data-state")).toBe("done");
    }
    expect(banner.getAttribute("data-state")).toBe("todo");
    expect(screen.queryByText(/KYC/i)).toBeNull();
  });
});
