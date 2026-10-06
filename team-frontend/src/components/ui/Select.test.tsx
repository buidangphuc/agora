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
  it("renders options and fires onChange with the picked value", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(
      <Select
        aria-label="Tỉnh"
        options={options}
        onChange={(e) => onChange(e.target.value)}
      />,
    );
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Tỉnh" }),
      "hcm",
    );
    expect(onChange).toHaveBeenCalledWith("hcm");
  });

  it("shows a disabled placeholder as the initial selection", () => {
    render(
      <Select aria-label="Tỉnh" placeholder="Chọn tỉnh" options={options} />,
    );
    expect(screen.getByRole("combobox")).toHaveValue("");
    expect(screen.getByRole("option", { name: "Chọn tỉnh" })).toBeDisabled();
  });

  it("disabled is announced; invalid sets aria-invalid", () => {
    const { rerender } = render(
      <Select aria-label="t" options={options} disabled />,
    );
    expect(screen.getByRole("combobox")).toBeDisabled();
    expect(screen.getByRole("combobox")).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    rerender(<Select aria-label="t" options={options} invalid />);
    expect(screen.getByRole("combobox")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
  });

  it("uses a keyboard-visible focus ring", () => {
    render(<Select aria-label="t" options={options} />);
    expect(screen.getByRole("combobox").className).toContain(
      "focus-visible:ring-2",
    );
  });

  it("is server-renderable", () => {
    expect(
      renderToStaticMarkup(<Select aria-label="t" options={options} />),
    ).toContain("<select");
  });
});
