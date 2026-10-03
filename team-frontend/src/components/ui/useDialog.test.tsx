import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { setupUser } from "@/test/user";

import { useDialog } from "./useDialog";

function Harness() {
  const [open, setOpen] = useState(false);
  const ref = useDialog<HTMLDialogElement>({
    open,
    onClose: () => setOpen(false),
  });
  return (
    <div>
      <button type="button" onClick={() => setOpen(true)}>
        open
      </button>
      <button type="button">outside</button>
      {open && (
        <dialog open ref={ref} aria-label="t">
          <button type="button">first</button>
          <button type="button">second</button>
          <button type="button">last</button>
        </dialog>
      )}
    </div>
  );
}

describe("useDialog", () => {
  it("moves focus into the dialog on open", async () => {
    const user = setupUser();
    render(<Harness />);
    await user.click(screen.getByText("open"));
    expect(screen.getByText("first")).toHaveFocus();
  });

  it("cycles Tab and Shift+Tab inside the dialog only", async () => {
    const user = setupUser();
    render(<Harness />);
    await user.click(screen.getByText("open"));
    await user.tab();
    expect(screen.getByText("second")).toHaveFocus();
    await user.tab();
    expect(screen.getByText("last")).toHaveFocus();
    await user.tab();
    expect(screen.getByText("first")).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByText("last")).toHaveFocus();
  });

  it("closes on Escape and returns focus to the opener", async () => {
    const user = setupUser();
    render(<Harness />);
    const opener = screen.getByText("open");
    await user.click(opener);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(opener).toHaveFocus();
  });

  it("locks body scroll while open and restores it after", async () => {
    const user = setupUser();
    document.body.style.overflow = "auto";
    render(<Harness />);
    await user.click(screen.getByText("open"));
    expect(document.body.style.overflow).toBe("hidden");
    await user.keyboard("{Escape}");
    expect(document.body.style.overflow).toBe("auto");
  });
});
