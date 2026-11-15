import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({
  request: vi.fn(),
  cancel: vi.fn(),
  toastSuccess: vi.fn(),
  refresh: vi.fn(),
}));
vi.mock("./actions", () => ({
  requestDeletionAction: h.request,
  cancelDeletionAction: h.cancel,
}));
vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({
    success: h.toastSuccess,
    error: vi.fn(),
    info: vi.fn(),
    showToast: vi.fn(),
  }),
}));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: h.refresh, push: vi.fn() }),
}));

import { DeletionCard } from "./DeletionCard";

const ERASE_AFTER = "2026-11-15T08:30:00.000Z";

beforeEach(() => {
  h.request.mockReset();
  h.cancel.mockReset();
  h.toastSuccess.mockReset();
  h.refresh.mockReset();
});
afterEach(cleanup);

describe("DeletionCard", () => {
  it("PV16 nothing pending offers Xóa tài khoản and opens the password modal", async () => {
    const user = userEvent.setup();
    render(<DeletionCard status={{ pending: false, eraseAfter: "" }} />);
    expect(
      screen.queryByRole("button", { name: /Hủy yêu cầu xóa/i }),
    ).toBeNull();
    await user.click(screen.getByRole("button", { name: "Xóa tài khoản" }));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText(/Mật khẩu/i)).toBeInTheDocument();
  });

  it("PV16 shows the pending panel with the vi-VN erase time after scheduling", async () => {
    const user = userEvent.setup();
    h.request.mockResolvedValue({
      ok: true,
      data: { eraseAfter: ERASE_AFTER },
    });
    h.refresh.mockImplementation(() => {});
    render(<DeletionCard status={{ pending: false, eraseAfter: "" }} />);
    await user.click(screen.getByRole("button", { name: "Xóa tài khoản" }));
    await user.type(screen.getByLabelText(/Mật khẩu/i), "mat-khau-dung");
    await user.click(screen.getByRole("button", { name: /Xác nhận/i }));
    expect(
      await screen.findByRole("button", { name: /Hủy yêu cầu xóa/i }),
    ).toBeInTheDocument();
    const when = new Date(ERASE_AFTER);
    expect(document.body.textContent).toContain(
      when.toLocaleDateString("vi-VN"),
    );
    expect(document.body.textContent).toContain(
      when.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit" }),
    );
    expect(document.body.textContent).not.toContain("ngày đã hẹn");
  });

  it("PV23 pending state hides the delete offer and shows the vi-VN erase time", () => {
    render(
      <DeletionCard status={{ pending: true, eraseAfter: ERASE_AFTER }} />,
    );
    expect(screen.queryByRole("button", { name: "Xóa tài khoản" })).toBeNull();
    expect(
      screen.getByRole("button", { name: /Hủy yêu cầu xóa/i }),
    ).toBeInTheDocument();
    const when = new Date(ERASE_AFTER);
    expect(document.body.textContent).toContain(
      when.toLocaleDateString("vi-VN"),
    );
  });

  it("PV21 cancel returns to the delete offer", async () => {
    const user = userEvent.setup();
    h.cancel.mockResolvedValue({ ok: true });
    render(
      <DeletionCard status={{ pending: true, eraseAfter: ERASE_AFTER }} />,
    );
    await user.click(screen.getByRole("button", { name: /Hủy yêu cầu xóa/i }));
    const dialog = screen.queryByRole("dialog");
    if (dialog) {
      const buttons = Array.from(dialog.querySelectorAll("button")).filter(
        (b) => /xác nhận|hủy yêu cầu xóa/i.test(b.textContent ?? ""),
      );
      await user.click(buttons[0]);
    }
    await waitFor(() => expect(h.cancel).toHaveBeenCalledTimes(1));
    expect(
      await screen.findByRole("button", { name: "Xóa tài khoản" }),
    ).toBeInTheDocument();
    expect(h.toastSuccess).toHaveBeenCalledWith(
      "Đã hủy yêu cầu xóa tài khoản.",
    );
  });
});
