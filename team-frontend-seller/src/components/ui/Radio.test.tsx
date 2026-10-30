import { render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Radio, RadioGroup } from "./Radio";

const options = [
  { value: "std", label: "Tiêu chuẩn" },
  { value: "fast", label: "Nhanh" },
  { value: "off", label: "Tắt", disabled: true },
];

describe("Radio", () => {
  it("a single Radio is labelled and selectable", async () => {
    const user = setupUser();
    render(<Radio name="x" label="Một" />);
    await user.click(screen.getByRole("radio", { name: "Một" }));
    expect(screen.getByRole("radio")).toBeChecked();
  });
});

describe("RadioGroup", () => {
  it("renders a group of radios sharing one name and reports the picked value", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <RadioGroup legend="Giao hàng" options={options} onChange={onChange} />,
    );
    expect(
      screen.getByRole("group", { name: "Giao hàng" }),
    ).toBeInTheDocument();
    const radios = screen.getAllByRole("radio");
    expect(new Set(radios.map((r) => r.getAttribute("name"))).size).toBe(1);
    await user.click(screen.getByRole("radio", { name: "Nhanh" }));
    expect(onChange).toHaveBeenCalledWith("fast");
  });

  it("arrow keys move the selection natively", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <RadioGroup
        legend="g"
        options={options}
        defaultValue="std"
        onChange={onChange}
      />,
    );
    screen.getByRole("radio", { name: "Tiêu chuẩn" }).focus();
    await user.keyboard("{ArrowDown}");
    expect(onChange).toHaveBeenLastCalledWith("fast");
    expect(screen.getByRole("radio", { name: "Nhanh" })).toBeChecked();
  });

  it("controlled value wins", async () => {
    const user = setupUser();
    function Harness() {
      const [v, setV] = useState("std");
      return (
        <RadioGroup legend="g" options={options} value={v} onChange={setV} />
      );
    }
    render(<Harness />);
    expect(screen.getByRole("radio", { name: "Tiêu chuẩn" })).toBeChecked();
    await user.click(screen.getByRole("radio", { name: "Nhanh" }));
    expect(screen.getByRole("radio", { name: "Nhanh" })).toBeChecked();
  });

  it("disabled options and a disabled group are inert and announced", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    const { rerender } = render(
      <RadioGroup legend="g" options={options} onChange={onChange} />,
    );
    expect(screen.getByRole("radio", { name: "Tắt" })).toBeDisabled();
    await user.click(screen.getByRole("radio", { name: "Tắt" }));
    expect(onChange).not.toHaveBeenCalled();

    rerender(
      <RadioGroup legend="g" options={options} onChange={onChange} disabled />,
    );
    expect(screen.getByRole("group")).toHaveAttribute("aria-disabled", "true");
    for (const r of screen.getAllByRole("radio")) {
      expect(r).toBeDisabled();
      expect(r).toHaveAttribute("aria-disabled", "true");
    }
  });
});
