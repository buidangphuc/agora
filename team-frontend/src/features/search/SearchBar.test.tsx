import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
  useSearchParams: () => new URLSearchParams(),
  redirect: vi.fn(),
  notFound: vi.fn(),
}));

import { SearchBar } from "./SearchBar";

const fetchMock = vi.fn();

beforeEach(() => {
  vi.useFakeTimers();
  push.mockClear();
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

async function type(input: HTMLElement, value: string) {
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(200);
  });
}

function suggest(list: string[]) {
  fetchMock.mockResolvedValue({
    ok: true,
    json: async () => ({ suggestions: list }),
  });
}

describe("SearchBar", () => {
  it("exposes combobox and listbox roles", async () => {
    suggest(["ao khoac", "ao thun"]);
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    await type(input, "ao");
    expect(input).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(2);
  });

  it("submits the typed query to /search?q=", () => {
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "ao khoac" } });
    fireEvent.submit(input.closest("form") as HTMLFormElement);
    expect(push).toHaveBeenCalledWith("/search?q=ao%20khoac");
  });

  it("ignores an empty submit", () => {
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    fireEvent.keyDown(input, { key: "Enter" });
    fireEvent.submit(input.closest("form") as HTMLFormElement);
    expect(push).not.toHaveBeenCalled();
  });

  it("debounces suggestion requests by 200ms", async () => {
    suggest([]);
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    fireEvent.change(input, { target: { value: "a" } });
    fireEvent.change(input, { target: { value: "ao" } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(199);
    });
    expect(fetchMock).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith("/api/suggest?q=ao");
  });

  it("selects the first suggestion with ArrowDown then Enter", async () => {
    suggest(["ao khoac", "ao thun"]);
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    await type(input, "ao");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input).toHaveAttribute(
      "aria-activedescendant",
      screen.getAllByRole("option")[0]?.id,
    );
    fireEvent.keyDown(input, { key: "Enter" });
    expect(push).toHaveBeenCalledWith("/search?q=ao%20khoac");
  });

  it("closes on Escape and wraps with ArrowUp", async () => {
    suggest(["a1", "a2"]);
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    await type(input, "a");
    fireEvent.keyDown(input, { key: "ArrowUp" });
    expect(screen.getAllByRole("option")[1]).toHaveAttribute(
      "aria-selected",
      "true",
    );
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input).toHaveAttribute("aria-expanded", "false");
  });

  it("shows trending keywords when empty and focused", () => {
    render(<SearchBar />);
    fireEvent.focus(screen.getByRole("combobox"));
    expect(screen.getByText("Tìm kiếm phổ biến")).toBeInTheDocument();
    expect(screen.getAllByRole("option").length).toBeGreaterThan(0);
  });

  it("hides suggestions silently when the request fails", async () => {
    fetchMock.mockRejectedValue(new Error("boom"));
    render(<SearchBar />);
    const input = screen.getByRole("combobox");
    await type(input, "ao");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.change(input, { target: { value: "ao dai" } });
    expect(input).toHaveValue("ao dai");
  });

  it("hides suggestions on a non-ok response", async () => {
    fetchMock.mockResolvedValue({ ok: false, json: async () => ({}) });
    render(<SearchBar />);
    await type(screen.getByRole("combobox"), "ao");
    expect(screen.queryByRole("listbox")).toBeNull();
  });
});
