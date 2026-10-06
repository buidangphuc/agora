import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const save = vi.hoisted(() => vi.fn());
const del = vi.hoisted(() => vi.fn());
const toast = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  showToast: vi.fn(),
}));

vi.mock("./actions", () => ({
  saveSearchAction: save,
  deleteSavedSearchAction: del,
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { SavedSearches } from "./SavedSearches";

beforeEach(() => vi.clearAllMocks());

describe("SavedSearches", () => {
  it("disables saving without a keyword", () => {
    render(<SavedSearches currentQuery="" initialSaved={[]} />);
    expect(
      screen.getByRole("button", { name: /Lưu tìm kiếm này/ }),
    ).toBeDisabled();
    expect(
      screen.getByText("Chưa có tìm kiếm nào được lưu."),
    ).toBeInTheDocument();
  });

  it("shows pending, then a success toast and the saved item", async () => {
    const gate: { resolve: (v: unknown) => void } = { resolve: () => {} };
    save.mockReturnValue(
      new Promise((resolve) => {
        gate.resolve = resolve;
      }),
    );
    render(<SavedSearches currentQuery="ao" initialSaved={[]} />);
    const button = screen.getByRole("button", { name: /Lưu tìm kiếm này/ });
    fireEvent.click(button);
    await waitFor(() => expect(button).toHaveAttribute("aria-busy", "true"));
    expect(button).toBeDisabled();
    gate.resolve({
      ok: true,
      data: { id: "1", query: "ao", filtersJson: "", createdAt: "" },
    });
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith("Đã lưu tìm kiếm."),
    );
    expect(screen.getByRole("link", { name: "ao" })).toBeInTheDocument();
    expect(button).not.toBeDisabled();
  });

  it("shows an error toast with the action error and re-enables the button", async () => {
    save.mockResolvedValue({ ok: false, error: "Lưu thất bại" });
    render(<SavedSearches currentQuery="ao" initialSaved={[]} />);
    const button = screen.getByRole("button", { name: /Lưu tìm kiếm này/ });
    fireEvent.click(button);
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Lưu thất bại"),
    );
    expect(button).not.toBeDisabled();
    expect(screen.queryByRole("link", { name: "ao" })).toBeNull();
  });

  it("restores a removed item when the delete fails", async () => {
    del.mockResolvedValue({ ok: false, error: "x" });
    render(
      <SavedSearches
        currentQuery=""
        initialSaved={[
          { id: "1", query: "ao", filtersJson: "", createdAt: "" },
        ]}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Xóa tìm kiếm ao" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(screen.getByRole("link", { name: "ao" })).toBeInTheDocument();
  });
});
