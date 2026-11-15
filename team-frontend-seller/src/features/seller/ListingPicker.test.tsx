import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { BundleManager } from "./BundleManager";
import { ListingPicker } from "./ListingPicker";
import { createBundleAction, searchMyListingsAction } from "./actions";

vi.mock("./actions", () => ({
  searchMyListingsAction: vi.fn(),
  createBundleAction: vi.fn(),
  createAdCampaignAction: vi.fn(),
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: vi.fn(), error: vi.fn() }),
}));

const items = (n: number, prefix = "Áo") =>
  Array.from({ length: n }, (_, i) => ({
    id: `${prefix}-${i}`,
    title: `${prefix} ${i}`,
  }));

const okPage = (list: { id: string; title: string }[]) =>
  ({
    ok: true,
    data: { page: { items: list, nextCursor: "", total: list.length } },
  }) as never;

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.mocked(searchMyListingsAction).mockImplementation((async (q: string) =>
    q === "quần"
      ? okPage(items(3, "Quần"))
      : q === "zzz"
        ? okPage([])
        : okPage(items(20))) as never);
});
afterEach(() => vi.useRealTimers());

async function flush(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

function typeText(q: string) {
  fireEvent.change(screen.getByLabelText(/Tìm sản phẩm/), {
    target: { value: q },
  });
}

describe("ListingPicker", () => {
  it("Picker opens with the first options: one empty-query search on mount lists at most 20", async () => {
    render(<ListingPicker selectedIds={[]} onToggle={vi.fn()} />);
    await flush();
    expect(searchMyListingsAction).toHaveBeenCalledTimes(1);
    expect(searchMyListingsAction).toHaveBeenCalledWith("");
    expect(screen.getAllByRole("checkbox")).toHaveLength(20);
  });

  it("Typing narrows the options: search fires after 300 ms of silence and replaces the options", async () => {
    render(<ListingPicker selectedIds={[]} onToggle={vi.fn()} />);
    await flush();
    typeText("q");
    typeText("quần");
    await flush(299);
    expect(searchMyListingsAction).toHaveBeenCalledTimes(1);
    await flush(1);
    expect(searchMyListingsAction).toHaveBeenCalledTimes(2);
    expect(searchMyListingsAction).toHaveBeenLastCalledWith("quần");
    expect(screen.getAllByRole("checkbox")).toHaveLength(3);
    expect(screen.getByLabelText("Quần 0")).toBeInTheDocument();
    expect(screen.queryByLabelText("Áo 0")).not.toBeInTheDocument();
  });

  it("Picker opens with the first options: choosing an option reports the listing", async () => {
    const onToggle = vi.fn();
    render(<ListingPicker selectedIds={[]} onToggle={onToggle} />);
    await flush();
    fireEvent.click(screen.getByLabelText("Áo 3"));
    expect(onToggle).toHaveBeenCalledWith({ id: "Áo-3", title: "Áo 3" });
  });

  it("Search without a result: shows the empty text and offers no option", async () => {
    render(<ListingPicker selectedIds={[]} onToggle={vi.fn()} />);
    await flush();
    typeText("zzz");
    await flush(300);
    expect(screen.getByText("Không tìm thấy sản phẩm")).toBeInTheDocument();
    expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  });

  it("Search fails: shows the error with a retry control, keeps the selection, retry recovers", async () => {
    render(<ListingPicker selectedIds={["Áo-1"]} onToggle={vi.fn()} />);
    await flush();
    vi.mocked(searchMyListingsAction).mockResolvedValueOnce({
      ok: false,
      error: "Không tải được danh sách sản phẩm.",
    } as never);
    typeText("áo");
    await flush(300);
    expect(screen.getByText("Không tải được sản phẩm")).toBeInTheDocument();
    const retry = screen.getByRole("button", { name: "Thử lại" });
    const callsBefore = vi.mocked(searchMyListingsAction).mock.calls.length;
    fireEvent.click(retry);
    await flush();
    expect(searchMyListingsAction).toHaveBeenCalledTimes(callsBefore + 1);
    expect(searchMyListingsAction).toHaveBeenLastCalledWith("áo");
    expect(screen.queryByText("Không tải được sản phẩm")).toBeNull();
    expect(screen.getByLabelText("Áo 1")).toBeChecked();
  });
});

describe("BundleManager with ListingPicker", () => {
  it("Selection survives a new search: two picked listings are still submitted by Tạo combo", async () => {
    vi.mocked(createBundleAction).mockResolvedValue({
      ok: true,
      data: { bundle: { id: "b" } },
    } as never);
    render(<BundleManager bundles={[]} />);
    await flush();
    fireEvent.click(screen.getByLabelText("Áo 1"));
    fireEvent.click(screen.getByLabelText("Áo 2"));
    typeText("quần");
    await flush(300);
    expect(screen.queryByLabelText("Áo 1")).not.toBeInTheDocument();
    expect(screen.getByText(/2 đã chọn/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Tên combo/), {
      target: { value: "Bộ" },
    });
    fireEvent.change(screen.getByLabelText(/Giá combo/), {
      target: { value: "100" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tạo combo" }));
    await waitFor(() =>
      expect(createBundleAction).toHaveBeenCalledWith(
        "Bộ",
        ["Áo-1", "Áo-2"],
        100,
      ),
    );
  });
});
