import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ListingStatus } from "@/generated/platform/listing/v1/listing_pb.js";
import { listMyListings } from "@/lib/gateway/listings";
import { getPrincipal } from "@/lib/gateway/session";
import { ListingPicker } from "./ListingPicker";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));
vi.mock("@/lib/gateway/listings", () => ({ listMyListings: vi.fn() }));
vi.mock("@/lib/gateway/session", () => ({
  getPrincipal: vi.fn(),
  hasScope: vi.fn(),
}));

const items = (n: number) =>
  Array.from({ length: n }, (_, i) => ({ id: `l${i}`, title: `Áo ${i}` }));

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.mocked(getPrincipal).mockReturnValue({
    id: "seller-a",
    name: "A",
    scopes: ["listing.write"],
  } as never);
  vi.mocked(listMyListings).mockResolvedValue({
    items: items(20),
    nextCursor: "more",
    total: 60,
  } as never);
});
afterEach(() => vi.useRealTimers());

async function type(q: string) {
  fireEvent.change(screen.getByLabelText(/Tìm sản phẩm/), {
    target: { value: q },
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(500);
  });
}

describe("ListingPicker", () => {
  it("Picker searches server-side: one ListMyListings call with query and PUBLISHED, no more pages, at most 20 options", async () => {
    render(<ListingPicker onSelect={vi.fn()} />);
    await type("ao");
    expect(listMyListings).toHaveBeenCalledTimes(1);
    const arg = vi.mocked(listMyListings).mock.calls[0][0];
    expect(arg).toMatchObject({
      query: "ao",
      statuses: [ListingStatus.PUBLISHED],
    });
    expect(arg?.cursor ?? "").toBe("");
    expect(arg?.pageSize ?? 20).toBeLessThanOrEqual(20);
    expect(screen.getAllByRole("button", { name: /Áo \d+/ }).length).toBe(20);
  });

  it("Picker searches server-side: typing is debounced into a single call", async () => {
    render(<ListingPicker onSelect={vi.fn()} />);
    const box = screen.getByLabelText(/Tìm sản phẩm/);
    fireEvent.change(box, { target: { value: "a" } });
    fireEvent.change(box, { target: { value: "ao" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(listMyListings).toHaveBeenCalledTimes(1);
    expect(vi.mocked(listMyListings).mock.calls[0][0]?.query).toBe("ao");
  });

  it("Picker searches server-side: choosing an option reports the listing", async () => {
    const onSelect = vi.fn();
    render(<ListingPicker onSelect={onSelect} />);
    await type("ao");
    fireEvent.click(screen.getByRole("button", { name: /Áo 3/ }));
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ id: "l3", title: "Áo 3" }),
    );
  });
});
