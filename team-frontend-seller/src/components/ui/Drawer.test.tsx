import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Drawer } from "./Drawer";

function Harness() {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        Mở bộ lọc
      </button>
      <Drawer
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Bộ lọc"
        footer={<button type="button">Áp dụng</button>}
      >
        <input aria-label="Giá tối đa" />
      </Drawer>
    </div>
  );
}

describe("Drawer", () => {
  it("renders nothing while closed", () => {
    render(
      <Drawer isOpen={false} onClose={() => {}} title="x">
        b
      </Drawer>,
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("is a labelled modal dialog", () => {
    render(
      <Drawer
        isOpen
        onClose={() => {}}
        title="Bộ lọc"
        description="Lọc sản phẩm"
      >
        b
      </Drawer>,
    );
    const dialog = screen.getByRole("dialog", { name: "Bộ lọc" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAccessibleDescription("Lọc sản phẩm");
  });

  it("traps focus, closes on Escape and returns focus to the opener", async () => {
    const user = setupUser();
    render(<Harness />);
    const opener = screen.getByRole("button", { name: "Mở bộ lọc" });
    await user.click(opener);
    const dialog = screen.getByRole("dialog");
    for (let i = 0; i < 6; i++) {
      await user.tab();
      expect(dialog.contains(document.activeElement)).toBe(true);
    }
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(opener).toHaveFocus();
  });

  it("closes from the close button and the backdrop", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Drawer isOpen onClose={onClose} title="T">
        b
      </Drawer>,
    );
    await user.click(screen.getByRole("button", { name: "Đóng" }));
    const backdrop = document.querySelector(
      '[data-state="open"]:not([role="dialog"])',
    );
    expect(backdrop).not.toBeNull();
    fireEvent.pointerDown(backdrop as Element);
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("names an untitled drawer", () => {
    render(
      <Drawer isOpen onClose={() => {}}>
        b
      </Drawer>,
    );
    expect(
      screen.getByRole("dialog", { name: "Ngăn kéo" }),
    ).toBeInTheDocument();
  });

  it("supports placement and locks scroll while open", () => {
    const { unmount } = render(
      <Drawer isOpen onClose={() => {}} title="T" placement="left">
        b
      </Drawer>,
    );
    expect(screen.getByRole("dialog")).toHaveClass("left-0");
    expect(document.body).toHaveAttribute("data-scroll-locked");
    unmount();
    expect(document.body).not.toHaveAttribute("data-scroll-locked");
  });
});
