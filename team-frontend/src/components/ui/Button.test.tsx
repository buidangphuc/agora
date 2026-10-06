import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Button } from "./Button";

describe("Button state contract", () => {
  it("a loading button keeps its label in flow, shows a spinner and blocks clicks", async () => {
    const onClick = vi.fn();
    const user = setupUser();
    const { rerender } = render(
      <Button onClick={onClick}>Thêm vào giỏ</Button>,
    );
    const idle = screen.getByRole("button", { name: "Thêm vào giỏ" });
    const idleLabel = idle.querySelector("span > span");
    expect(idle).not.toHaveAttribute("aria-busy");

    rerender(
      <Button onClick={onClick} isLoading>
        Thêm vào giỏ
      </Button>,
    );
    const busy = screen.getByRole("button", { name: "Thêm vào giỏ" });
    expect(busy).toHaveAttribute("aria-busy", "true");
    // Width preserved: the label is still rendered (hidden by opacity, not removed).
    expect(busy.querySelector("span > span")?.textContent).toBe(
      idleLabel?.textContent,
    );
    expect(busy.firstElementChild).toHaveClass("opacity-0");
    // Spinner is an overlay, not an extra flow item.
    expect(busy.querySelector(".animate-spin")).not.toBeNull();
    expect(busy.querySelector(".absolute")).not.toBeNull();

    expect(busy).toBeDisabled();
    expect(busy).toHaveAttribute("aria-disabled", "true");
    await user.click(busy);
    expect(onClick).not.toHaveBeenCalled();
  });

  it("a loading submit button does not submit its form", async () => {
    const onSubmit = vi.fn((e: React.FormEvent) => e.preventDefault());
    const user = setupUser();
    render(
      <form onSubmit={onSubmit}>
        <Button type="submit" isLoading>
          Gửi
        </Button>
      </form>,
    );
    await user.click(screen.getByRole("button"));
    await user.keyboard("{Enter}");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("disabled is announced, inert for clicks and for the keyboard", async () => {
    const onClick = vi.fn();
    const user = setupUser();
    render(
      <Button disabled onClick={onClick}>
        Mua
      </Button>,
    );
    const btn = screen.getByRole("button", { name: "Mua" });
    expect(btn).toBeDisabled();
    expect(btn).toHaveAttribute("aria-disabled", "true");
    expect(btn).toHaveClass("opacity-50", "pointer-events-none");
    await user.click(btn);
    btn.focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onClick).not.toHaveBeenCalled();
  });

  it("an enabled button calls onClick and is not aria-disabled", async () => {
    const onClick = vi.fn();
    const user = setupUser();
    render(<Button onClick={onClick}>Mua</Button>);
    const btn = screen.getByRole("button", { name: "Mua" });
    expect(btn).not.toHaveAttribute("aria-disabled");
    await user.click(btn);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("uses a keyboard-only focus ring, never a bare focus: ring", () => {
    render(<Button>Mua</Button>);
    const cls = screen.getByRole("button").className;
    expect(cls).toContain("focus-visible:ring-2");
    expect(cls).toContain("focus-visible:ring-focus-ring");
    expect(cls).not.toMatch(/(^|\s)focus:/);
    expect(cls).toContain("active:scale-95");
  });

  it("keeps variants, sizes, icons and ref compatible", () => {
    const ref = { current: null as HTMLButtonElement | null };
    render(
      <Button
        ref={ref}
        variant="danger"
        size="lg"
        leftIcon={<i>L</i>}
        rightIcon={<i>R</i>}
        className="extra"
      >
        Xoá
      </Button>,
    );
    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
    expect(ref.current).toHaveClass("bg-danger", "extra");
    expect(screen.getByText("L")).toBeInTheDocument();
    expect(screen.getByText("R")).toBeInTheDocument();
  });
});
