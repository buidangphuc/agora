import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { Checkbox } from "./Checkbox";

describe("Checkbox facade", () => {
  it("is labelled and toggles on click and Space", async () => {
    const onCheckedChange = vi.fn();
    render(<Checkbox label="Nhớ tôi" onCheckedChange={onCheckedChange} />);
    const box = screen.getByRole("checkbox", { name: "Nhớ tôi" });
    await userEvent.click(box);
    expect(onCheckedChange).toHaveBeenLastCalledWith(true);
    await userEvent.keyboard(" ");
    expect(onCheckedChange).toHaveBeenLastCalledWith(false);
  });

  it("clicking the label toggles it", async () => {
    render(<Checkbox label="Nhớ tôi" />);
    await userEvent.click(screen.getByText("Nhớ tôi"));
    expect(screen.getByRole("checkbox", { name: "Nhớ tôi" })).toBeChecked();
  });

  it("supports controlled checked and the mixed state", () => {
    const { rerender } = render(<Checkbox label="Tất cả" checked />);
    expect(screen.getByRole("checkbox")).toBeChecked();
    rerender(<Checkbox label="Tất cả" checked="indeterminate" />);
    expect(screen.getByRole("checkbox")).toBePartiallyChecked();
  });

  it("a hidden label still names the box", () => {
    render(<Checkbox label="Chọn sản phẩm" labelHidden />);
    expect(
      screen.getByRole("checkbox", { name: "Chọn sản phẩm" }),
    ).toBeInTheDocument();
  });

  it("disabled is announced and inert", async () => {
    const onCheckedChange = vi.fn();
    render(
      <Checkbox label="Nhớ tôi" disabled onCheckedChange={onCheckedChange} />,
    );
    const box = screen.getByRole("checkbox", { name: "Nhớ tôi" });
    expect(box).toBeDisabled();
    await userEvent.click(box);
    expect(onCheckedChange).not.toHaveBeenCalled();
  });

  it("submits its value inside a form", () => {
    render(
      <form data-testid="f">
        <Checkbox label="Mặc định" name="isDefault" defaultChecked />
      </form>,
    );
    const data = new FormData(screen.getByTestId("f") as HTMLFormElement);
    expect(data.get("isDefault")).toBe("on");
  });
});
