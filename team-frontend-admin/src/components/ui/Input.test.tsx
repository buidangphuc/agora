import { render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it } from "vitest";

import { Input } from "./Input";

describe("Input facade", () => {
  it("links the label with htmlFor", () => {
    render(<Input label="Email" />);
    expect(screen.getByLabelText("Email").tagName).toBe("INPUT");
  });

  it("an error is announced: aria-invalid plus an accessible description", () => {
    render(<Input label="Email" error="Email không hợp lệ" />);
    const input = screen.getByLabelText("Email");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(input).toHaveAccessibleDescription("Email không hợp lệ");
  });

  it("helper text is the description when there is no error", () => {
    render(<Input label="Email" helperText="Dùng email công ty" />);
    expect(screen.getByLabelText("Email")).toHaveAccessibleDescription(
      "Dùng email công ty",
    );
  });

  it("disabled is announced", () => {
    render(<Input label="Email" disabled />);
    expect(screen.getByLabelText("Email")).toBeDisabled();
  });

  it("marks required without changing the accessible name", () => {
    render(<Input label="Email" required />);
    expect(screen.getByRole("textbox", { name: "Email" })).toBeRequired();
  });

  it("forwards the ref and native props", () => {
    const ref = createRef<HTMLInputElement>();
    render(<Input label="Giá" ref={ref} name="price" type="number" />);
    expect(ref.current?.name).toBe("price");
    expect(ref.current?.type).toBe("number");
  });

  it("is at least the 44px touch height", () => {
    render(<Input label="Email" />);
    expect(screen.getByLabelText("Email").className).toContain("min-h-touch");
  });
});
