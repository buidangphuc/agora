import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { ViewListing } from "@/lib/gateway/listings";

vi.mock("@/lib/analytics", () => ({ trackEcommerce: vi.fn() }));
vi.mock("@/features/engagement/FavoriteButton", () => ({
  FavoriteButton: () => null,
}));

import { CountdownClock, formatRemaining } from "./CountdownClock";
import { FlashSaleSection } from "./FlashSaleSection";

function listing(id: string): ViewListing {
  return {
    id,
    title: `SP ${id}`,
    description: "",
    price: 100000,
    currency: "VND",
    status: "published",
    sellerId: "s",
    imageKeys: [],
    imageUrl: "https://img.test/x.jpg",
    categoryId: "c",
    stock: 1,
    variants: [],
  };
}

afterEach(() => vi.useRealTimers());

describe("FlashSaleSection", () => {
  it("renders nothing for an empty list", () => {
    const { container } = render(<FlashSaleSection listings={[]} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("shows no countdown, sold bar or discount without real campaign data", () => {
    const { container } = render(
      <FlashSaleSection listings={[listing("a"), listing("b")]} />,
    );
    expect(screen.getByText("Flash Sale")).toBeInTheDocument();
    expect(screen.queryByTestId("countdown-clock")).toBeNull();
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(container.textContent).not.toMatch(/ĐÃ BÁN|đã bán|-\d+%/i);
  });

  it("shows Progress only for items with real sold/stock", () => {
    const { container } = render(
      <FlashSaleSection
        listings={[listing("a"), listing("b")]}
        stockById={{ a: { sold: 30, stock: 120 } }}
      />,
    );
    const bars = container.querySelectorAll("progress");
    expect(bars).toHaveLength(1);
    expect(bars[0]).toHaveAttribute("value", "25");
  });

  it("is a server component: only the clock is a client module", () => {
    const section = readFileSync(
      resolve(process.cwd(), "src/features/home/FlashSaleSection.tsx"),
      "utf8",
    );
    const clock = readFileSync(
      resolve(process.cwd(), "src/features/home/CountdownClock.tsx"),
      "utf8",
    );
    expect(section).not.toMatch(/["']use client["']/);
    expect(clock.trimStart().startsWith('"use client"')).toBe(true);
  });
});

describe("CountdownClock", () => {
  it("formats remaining time and never goes negative", () => {
    expect(formatRemaining(3_600_000, 1_000)).toBe("00:59:59");
    expect(formatRemaining(1_000, 5_000)).toBe("00:00:00");
  });

  it("ticks without changing its box", () => {
    vi.useFakeTimers();
    vi.setSystemTime(0);
    render(<CountdownClock endsAt={3_601_000} />);
    const clock = screen.getByTestId("countdown-clock");
    const before = clock.className;
    expect(clock.textContent).toBe("01:00:01");
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(clock.textContent).toBe("00:59:59");
    expect(clock.className).toBe(before);
    expect(clock.className).toMatch(/tabular-nums/);
    expect(clock.className).toMatch(/min-w-/);
    expect(clock.textContent).toHaveLength(8);
  });
});
