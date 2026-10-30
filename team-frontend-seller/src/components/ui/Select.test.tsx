import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Select } from "./Select";

const options = [
  { value: "hn", label: "Hà Nội" },
  { value: "hcm", label: "TP.HCM" },
  { value: "dn", label: "Đà Nẵng", disabled: true },
];

describe("Select", () => {
  it("renders a labelled native select and fires onChange with the picked value", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <Select
        label="Tỉnh"
        options={options}
        onChange={(e) => onChange(e.target.value)}
      />,
    );
    const select = screen.getByRole("combobox", { name: "Tỉnh" });
    expect(select.tagName).toBe("SELECT");
    await user.selectOptions(select, "hcm");
    expect(onChange).toHaveBeenCalledWith("hcm");
    expect(screen.getByRole("option", { name: "Đà Nẵng" })).toBeDisabled();
  });

  it("shows a disabled placeholder as the initial selection", () => {
    render(<Select label="Tỉnh" placeholder="Chọn tỉnh" options={options} />);
    expect(screen.getByRole("combobox")).toHaveValue("");
    expect(screen.getByRole("option", { name: "Chọn tỉnh" })).toBeDisabled();
  });

  it("maps helperText and error onto the description and invalid state", () => {
    render(
      <Select
        label="Tỉnh"
        options={options}
        helperText="Nơi nhận hàng"
        error="Vui lòng chọn tỉnh"
      />,
    );
    const select = screen.getByRole("combobox", { name: "Tỉnh" });
    expect(select).toHaveAttribute("aria-invalid", "true");
    expect(select).toHaveAccessibleDescription(
      "Vui lòng chọn tỉnh Nơi nhận hàng",
    );
  });

  it("keeps id, name, data-testid and disabled on the select", () => {
    render(
      <Select
        label="Lý do"
        id="reason"
        name="reason"
        data-testid="return-reason"
        options={options}
        disabled
      />,
    );
    const select = screen.getByTestId("return-reason");
    expect(select).toBe(screen.getByRole("combobox", { name: "Lý do" }));
    expect(select).toHaveAttribute("id", "reason");
    expect(select).toHaveAttribute("name", "reason");
    expect(select).toBeDisabled();
  });

  it("names the select when the label is hidden", () => {
    render(<Select label="Sắp xếp" labelHidden options={options} />);
    expect(screen.getByRole("combobox", { name: "Sắp xếp" })).toBeVisible();
  });

  it("is server-renderable", () => {
    expect(
      renderToStaticMarkup(<Select label="t" options={options} />),
    ).toContain("<select");
  });
});
