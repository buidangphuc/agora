import { render, screen, waitFor } from "@testing-library/react";
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

  it("is a labelled modal dialog with its footer inside", () => {
    render(
      <Modal
        isOpen
        onClose={() => {}}
        title="Chọn địa chỉ"
        description="Mô tả"
        footer={<button type="button">Xác nhận</button>}
      >
        body
      </Modal>,
    );
    const dialog = screen.getByRole("dialog", { name: "Chọn địa chỉ" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAccessibleDescription("Mô tả");
    expect(dialog).toContainElement(
      screen.getByRole("button", { name: "Xác nhận" }),
    );
  });

  it("traps focus, closes on Escape and returns focus to the opener", async () => {
    const user = setupUser();
    const spy = vi.fn();
    render(<Harness onCloseSpy={spy} />);
    const opener = screen.getByRole("button", { name: "Mở" });
    await user.click(opener);

    const dialog = screen.getByRole("dialog");
    await waitFor(() =>
      expect(dialog.contains(document.activeElement)).toBe(true),
    );

    for (let i = 0; i < 8; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    await user.tab({ shift: true });
    expect(dialog.contains(document.activeElement)).toBe(true);

    await user.keyboard("{Escape}");
    expect(spy).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("dialog")).toBeNull();
    await waitFor(() => expect(opener).toHaveFocus());
  });

  it("closes from the close button", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Modal isOpen onClose={onClose} title="T">
        b
      </Modal>,
    );
    await user.click(screen.getByRole("button", { name: "Đóng" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("closes from the overlay", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Modal isOpen onClose={onClose} title="T">
        b
      </Modal>,
    );
    const overlay = document.body.querySelector<HTMLElement>(
      'div[data-state="open"]',
    );
    expect(overlay).not.toBeNull();
    await user.click(overlay as HTMLElement);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("stays open when onClose ignores the request", async () => {
    const user = setupUser();
    render(
      <Modal isOpen onClose={() => {}} title="T">
        b
      </Modal>,
    );
    await user.keyboard("{Escape}");
    expect(screen.getByRole("dialog", { name: "T" })).toBeInTheDocument();
  });
});
