import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
  it("is labelled and toggles on click and Space", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Checkbox label="Nhớ tôi" onChange={onChange} />);
    const box = screen.getByRole("checkbox", { name: "Nhớ tôi" });
    await user.click(box);
    expect(box).toBeChecked();
    await user.keyboard(" ");
    expect(box).not.toBeChecked();
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it("clicking the label toggles it and a description is shown", async () => {
    const user = setupUser();
    render(<Checkbox label="Đồng ý" description="Điều khoản" />);
    await user.click(screen.getByText("Đồng ý"));
    expect(screen.getByRole("checkbox")).toBeChecked();
    expect(screen.getByText("Điều khoản")).toBeInTheDocument();
  });

  it("supports controlled checked", () => {
    render(<Checkbox label="A" checked readOnly />);
    expect(screen.getByRole("checkbox")).toBeChecked();
  });

  it("disabled is announced and inert", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Checkbox label="A" disabled onChange={onChange} />);
    const box = screen.getByRole("checkbox");
    expect(box).toBeDisabled();
    expect(box).toHaveAttribute("aria-disabled", "true");
    await user.click(box);
    await user.keyboard(" ");
    expect(onChange).not.toHaveBeenCalled();
  });

  it("has a keyboard-visible focus ring", () => {
    render(<Checkbox label="A" />);
    expect(screen.getByRole("checkbox").className).toContain(
      "focus-visible:ring-2",
    );
  });
});
