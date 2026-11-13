import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FeaturedListingsPicker } from "./FeaturedListingsPicker";
import { searchMyListingsAction } from "./actions";

vi.mock("./actions", () => ({ searchMyListingsAction: vi.fn() }));

const CAP = "Tối đa 6 sản phẩm nổi bật.";
const FAIL = "Không tải được danh sách sản phẩm.";
const item = (id: string, title = `Sản phẩm ${id}`) => ({ id, title });
const page = (...ids: string[]) =>
  ({
    ok: true,
    data: {
      page: {
        items: ids.map((i) => item(i)),
        nextCursor: "",
        total: ids.length,
      },
    },
  }) as never;

let selection: string[] = [];

function Harness({
  initial,
  titles = {},
}: {
  initial: string[];
  titles?: Record<string, string>;
}) {
  const [v, setV] = useState(initial);
  selection = v;
  return (
    <FeaturedListingsPicker
      value={v}
      titles={titles}
      onChange={(ids: string[]) => {
        selection = ids;
        setV(ids);
      }}
    />
  );
}

const titlesFor = (ids: string[]) =>
  Object.fromEntries(ids.map((i) => [i, `Sản phẩm ${i}`]));

async function type(q: string) {
  fireEvent.change(screen.getByLabelText(/Tìm sản phẩm/), {
    target: { value: q },
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(300);
  });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useFakeTimers();
  selection = [];
});
afterEach(() => vi.useRealTimers());

describe("FeaturedListingsPicker", () => {
  it("Seller shop UI: search runs on the server with the typed query — search is one filtered call", async () => {
    vi.mocked(searchMyListingsAction).mockResolvedValue(page("a", "b"));
    render(<Harness initial={[]} />);
    fireEvent.change(screen.getByLabelText(/Tìm sản phẩm/), {
      target: { value: "áo" },
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(299);
    });
    expect(searchMyListingsAction).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(searchMyListingsAction).toHaveBeenCalledTimes(1);
    expect(searchMyListingsAction).toHaveBeenCalledWith("áo");
    expect(screen.getByText("Sản phẩm a")).toBeInTheDocument();
  });

  it("Seller shop UI: the sixth featured listing is accepted — sixth accepted", async () => {
    const five = ["a", "b", "c", "d", "e"];
    vi.mocked(searchMyListingsAction).mockResolvedValue(page("f"));
    render(<Harness initial={five} titles={titlesFor(five)} />);
    await type("f");
    fireEvent.click(screen.getByRole("button", { name: /^Thêm/ }));
    expect(selection).toEqual([...five, "f"]);
  });

  it("Seller shop UI: the seventh featured listing is refused — cap of six", async () => {
    const six = ["a", "b", "c", "d", "e", "f"];
    vi.mocked(searchMyListingsAction).mockResolvedValue(page("g"));
    render(<Harness initial={six} titles={titlesFor(six)} />);
    await type("g");
    expect(screen.getByRole("button", { name: /^Thêm/ })).toBeDisabled();
    expect(screen.getByText(CAP)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^Thêm/ }));
    expect(selection).toEqual(six);
  });

  it("Seller shop UI: an already selected listing is not added twice — no duplicates", async () => {
    vi.mocked(searchMyListingsAction).mockResolvedValue(page("a"));
    render(<Harness initial={["a"]} titles={titlesFor(["a"])} />);
    await type("a");
    const add = screen.getByRole("button", { name: /^Thêm/ });
    fireEvent.click(add);
    expect(selection).toEqual(["a"]);
  });

  it("Seller shop UI: a search failure leaves the selection intact — search failure keeps selection", async () => {
    vi.mocked(searchMyListingsAction).mockResolvedValue({
      ok: false,
      error: FAIL,
    } as never);
    render(<Harness initial={["a", "b"]} titles={titlesFor(["a", "b"])} />);
    await type("x");
    expect(screen.getByText(FAIL)).toBeInTheDocument();
    expect(selection).toEqual(["a", "b"]);
    expect(screen.getByText("Sản phẩm a")).toBeInTheDocument();
    vi.mocked(searchMyListingsAction).mockResolvedValue(page("c"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(searchMyListingsAction).toHaveBeenCalledTimes(2);
    expect(screen.getByText("Sản phẩm c")).toBeInTheDocument();
    expect(selection).toEqual(["a", "b"]);
  });

  it("Seller shop UI: an unavailable featured listing can be removed — unavailable featured can be removed", () => {
    render(<Harness initial={["a", "x"]} titles={titlesFor(["a"])} />);
    expect(screen.getByText("Không còn khả dụng")).toBeInTheDocument();
    const row = screen.getByText("Không còn khả dụng").closest("li");
    if (!row) throw new Error("row not found");
    fireEvent.click(within(row).getByRole("button", { name: /^Xóa/ }));
    expect(selection).toEqual(["a"]);
    expect(screen.queryByText("Không còn khả dụng")).toBeNull();
  });

  it("Seller shop UI: reordering changes the saved order — move up and down", () => {
    const ids = ["a", "b", "c"];
    render(<Harness initial={ids} titles={titlesFor(ids)} />);
    fireEvent.click(screen.getByRole("button", { name: "Lên Sản phẩm c" }));
    expect(selection).toEqual(["a", "c", "b"]);
    fireEvent.click(screen.getByRole("button", { name: "Xuống Sản phẩm a" }));
    expect(selection).toEqual(["c", "a", "b"]);
  });
});
