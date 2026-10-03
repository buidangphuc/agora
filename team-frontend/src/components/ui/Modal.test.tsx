import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Modal } from "./Modal";

function Harness({ onCloseSpy }: { onCloseSpy?: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        Mở
      </button>
      <Modal
        isOpen={open}
        onClose={() => {
          onCloseSpy?.();
          setOpen(false);
        }}
        title="Chọn địa chỉ"
        description="Địa chỉ giao hàng"
        footer={<button type="button">Xác nhận</button>}
      >
        <input aria-label="Tên" />
      </Modal>
    </div>
  );
}

describe("Modal", () => {
  it("renders nothing while closed", () => {
    render(
      <Modal isOpen={false} onClose={() => {}} title="x">
        body
      </Modal>,
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("is a labelled modal dialog", () => {
    render(
      <Modal isOpen onClose={() => {}} title="Chọn địa chỉ" description="Mô tả">
        body
      </Modal>,
    );
    const dialog = screen.getByRole("dialog", { name: "Chọn địa chỉ" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAccessibleDescription("Mô tả");
  });

  it("traps focus, closes on Escape and returns focus to the opener", async () => {
    const user = setupUser();
    const spy = vi.fn();
    render(<Harness onCloseSpy={spy} />);
    const opener = screen.getByRole("button", { name: "Mở" });
    await user.click(opener);

    const dialog = screen.getByRole("dialog");
    const close = screen.getAllByRole("button", { name: "Đóng" })[1];
    expect(close).toHaveFocus(); // first tabbable inside the dialog

    // Tab many times: focus never leaves the dialog.
    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    await user.tab({ shift: true });
    expect(dialog.contains(document.activeElement)).toBe(true);

    await user.keyboard("{Escape}");
    expect(spy).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(opener).toHaveFocus();
  });

  it("closes from the close button and the backdrop", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Modal isOpen onClose={onClose} title="T">
        b
      </Modal>,
    );
    const [backdrop, closeBtn] = screen.getAllByRole("button", {
      name: "Đóng",
    });
    await user.click(closeBtn);
    await user.click(backdrop);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("locks scroll while open", () => {
    document.body.style.overflow = "";
    const { unmount } = render(
      <Modal isOpen onClose={() => {}} title="T">
        b
      </Modal>,
    );
    expect(document.body.style.overflow).toBe("hidden");
    unmount();
    expect(document.body.style.overflow).toBe("");
  });

  it("falls back to an aria-label when there is no title", () => {
    render(
      <Modal isOpen onClose={() => {}}>
        b
      </Modal>,
    );
    expect(
      screen.getByRole("dialog", { name: "Hộp thoại" }),
    ).toBeInTheDocument();
  });
});
