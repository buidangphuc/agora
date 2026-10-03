import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Checkbox } from "./Checkbox";
import { FormItem } from "./FormItem";
import { RadioGroup } from "./Radio";
import { Select } from "./Select";

describe("FormItem", () => {
  it("binds the label to the control and shows the required mark", () => {
    render(
      <FormItem label="Tỉnh/Thành" required>
        <Select options={[{ value: "hn", label: "Hà Nội" }]} />
      </FormItem>,
    );
    expect(screen.getByLabelText(/Tỉnh\/Thành/)).toBe(
      screen.getByRole("combobox"),
    );
    expect(screen.getByText("*")).toHaveAttribute("aria-hidden", "true");
  });

  it("links help text and announces an error status", () => {
    render(
      <FormItem label="Tỉnh" help="Vui lòng chọn tỉnh" status="error">
        <Select options={[]} />
      </FormItem>,
    );
    const select = screen.getByRole("combobox");
    expect(select).toHaveAttribute("aria-invalid", "true");
    expect(select).toHaveAccessibleDescription("Vui lòng chọn tỉnh");
    expect(screen.getByText("Vui lòng chọn tỉnh")).toHaveClass("text-danger");
  });

  it("non-error status leaves aria-invalid off and keeps existing describedby", () => {
    render(
      <>
        <p id="x">Ghi chú</p>
        <FormItem label="Tỉnh" help="Ok" status="success">
          <Select aria-describedby="x" options={[]} />
        </FormItem>
      </>,
    );
    const select = screen.getByRole("combobox");
    expect(select).not.toHaveAttribute("aria-invalid");
    expect(select).toHaveAccessibleDescription("Ghi chú Ok");
  });

  it("group mode names a RadioGroup via aria-labelledby", () => {
    render(
      <FormItem label="Giao hàng" group help="Chọn một">
        <RadioGroup
          options={[
            { value: "a", label: "Nhanh" },
            { value: "b", label: "Tiết kiệm" },
          ]}
        />
      </FormItem>,
    );
    const group = screen.getByRole("group", { name: "Giao hàng" });
    expect(group).toHaveAccessibleDescription("Chọn một");
  });

  it("keeps a control's own id and works with a non-element child", () => {
    render(
      <FormItem label="Đồng ý">
        <Checkbox id="my-id" label="Tôi đồng ý" />
      </FormItem>,
    );
    expect(document.getElementById("my-id")).toBeInTheDocument();
    render(<FormItem label="Chỉ chữ">text only</FormItem>);
    expect(screen.getByText("Chỉ chữ")).toBeInTheDocument();
  });
});
