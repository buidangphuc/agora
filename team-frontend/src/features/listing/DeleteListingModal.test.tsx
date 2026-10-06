import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { DeleteListingModal } from "./DeleteListingModal";
import { deleteListingAction } from "./actions";

vi.mock("./actions", () => ({ deleteListingAction: vi.fn() }));
const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({ success: toastSuccess, error: toastError }),
}));

function open() {
  render(<DeleteListingModal id="l1" title="Áo thun" />);
  fireEvent.click(screen.getByRole("button", { name: "Xoá Áo thun" }));
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(window, "confirm");
});

describe("DeleteListingModal", () => {
  it("asks for confirmation, focused on Cancel, and deletes nothing yet", () => {
    open();
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAccessibleName('Xoá "Áo thun"?');
    expect(screen.getByText("Hành động này không thể hoàn tác.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Huỷ" })).toHaveFocus();
    expect(deleteListingAction).not.toHaveBeenCalled();
    expect(window.confirm).not.toHaveBeenCalled();
  });

  it("shows pending then closes with a success toast", async () => {
    let resolve: (v: { ok: true }) => void = () => {};
    vi.mocked(deleteListingAction).mockReturnValue(
      new Promise((r) => {
        resolve = r;
      }),
    );
    open();
    fireEvent.click(screen.getByRole("button", { name: "Xoá sản phẩm" }));

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Xoá sản phẩm" }),
      ).toBeDisabled(),
    );
    expect(
      screen.getByRole("button", { name: "Xoá sản phẩm" }),
    ).toHaveAttribute("aria-busy", "true");
    expect(screen.getByRole("button", { name: "Huỷ" })).toBeDisabled();
    expect(deleteListingAction).toHaveBeenCalledWith("l1");

    resolve({ ok: true });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(toastSuccess).toHaveBeenCalledWith("Đã xoá sản phẩm");
  });

  it("keeps the Modal open with an Alert and error toast on failure", async () => {
    vi.mocked(deleteListingAction).mockResolvedValue({
      ok: false,
      error: "Không xoá được",
    });
    open();
    fireEvent.click(screen.getByRole("button", { name: "Xoá sản phẩm" }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent("Không xoá được"),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(toastError).toHaveBeenCalledWith("Không xoá được");
    expect(screen.getByRole("button", { name: "Xoá sản phẩm" })).toBeEnabled();
  });

  it("Cancel closes without deleting", () => {
    open();
    fireEvent.click(screen.getByRole("button", { name: "Huỷ" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(deleteListingAction).not.toHaveBeenCalled();
  });
});
