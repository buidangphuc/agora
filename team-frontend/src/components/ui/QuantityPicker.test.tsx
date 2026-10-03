import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { QuantityPicker } from "./QuantityPicker";

describe("QuantityPicker", () => {
  it("respects its bounds: increment disabled at max, typing clamps, decrement stays enabled", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <QuantityPicker min={1} max={3} defaultValue={3} onChange={onChange} />,
    );
    const input = screen.getByRole("spinbutton", { name: "Số lượng" });
    expect(input).toHaveValue("3");
    expect(
      screen.getByRole("button", { name: "Tăng số lượng" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "Giảm số lượng" })).toBeEnabled();

    await user.clear(input);
    await user.type(input, "5");
    expect(input).toHaveValue("3");
    expect(input).toHaveAttribute("aria-valuenow", "3");
    expect(screen.getByRole("button", { name: "Giảm số lượng" })).toBeEnabled();
  });

  it("decrement disables at min and steppers move by step", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <QuantityPicker
        min={1}
        max={10}
        step={2}
        defaultValue={3}
        onChange={onChange}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Giảm số lượng" }));
    expect(onChange).toHaveBeenLastCalledWith(1);
    expect(
      screen.getByRole("button", { name: "Giảm số lượng" }),
    ).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    expect(onChange).toHaveBeenLastCalledWith(3);
  });

  it("clamps up to min on blur when the typed value is below it", async () => {
    const user = setupUser();
    render(<QuantityPicker min={5} max={20} defaultValue={8} />);
    const input = screen.getByRole("spinbutton");
    await user.clear(input);
    await user.type(input, "2");
    await user.tab();
    expect(input).toHaveValue("5");
  });

  it("lets multi-digit values be typed when the first digit is below min", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <QuantityPicker min={5} max={50} defaultValue={5} onChange={onChange} />,
    );
    const input = screen.getByRole("spinbutton");
    await user.clear(input);
    await user.type(input, "12");
    await user.tab();
    expect(input).toHaveValue("12");
    expect(onChange).toHaveBeenLastCalledWith(12);
  });

  it("ArrowUp / ArrowDown step the value", async () => {
    const user = setupUser();
    render(<QuantityPicker min={1} max={5} defaultValue={2} />);
    const input = screen.getByRole("spinbutton");
    input.focus();
    await user.keyboard("{ArrowUp}");
    expect(input).toHaveValue("3");
    await user.keyboard("{ArrowDown}{ArrowDown}{ArrowDown}");
    expect(input).toHaveValue("1");
  });

  it("works controlled", async () => {
    const user = setupUser();
    function Harness() {
      const [v, setV] = useState(1);
      return <QuantityPicker value={v} max={4} onChange={setV} />;
    }
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    expect(screen.getByRole("spinbutton")).toHaveValue("2");
  });

  it("disabled is announced and inert", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<QuantityPicker disabled defaultValue={2} onChange={onChange} />);
    const input = screen.getByRole("spinbutton");
    expect(input).toBeDisabled();
    expect(input).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByRole("button", { name: "Tăng số lượng" }),
    ).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Tăng số lượng" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});
