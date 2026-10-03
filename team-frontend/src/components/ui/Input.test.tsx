import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Input } from "./Input";

describe("Input a11y", () => {
  it("links the label with htmlFor", () => {
    render(<Input label="Email" />);
    const input = screen.getByLabelText("Email");
    expect(input.tagName).toBe("INPUT");
  });

  it("an error is announced: aria-invalid plus an accessible description", () => {
    render(<Input label="Email" error="Email không hợp lệ" />);
    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAccessibleDescription("Email không hợp lệ");
  });

  it("helper text is the description when there is no error", () => {
    render(<Input label="Tên" helperText="Tối đa 50 ký tự" />);
    const input = screen.getByLabelText("Tên");
    expect(input).not.toHaveAttribute("aria-invalid");
    expect(input).toHaveAccessibleDescription("Tối đa 50 ký tự");
  });

  it("keeps a caller-supplied aria-describedby alongside the message", () => {
    render(
      <>
        <p id="extra">Gợi ý</p>
        <Input label="Mã" aria-describedby="extra" error="Sai" />
      </>,
    );
    expect(screen.getByLabelText("Mã")).toHaveAccessibleDescription(
      "Gợi ý Sai",
    );
  });

  it("disabled is announced", () => {
    render(<Input label="Mã" disabled />);
    const input = screen.getByLabelText("Mã");
    expect(input).toBeDisabled();
    expect(input).toHaveAttribute("aria-disabled", "true");
  });

  it("uses a keyboard-visible focus ring and marks required", () => {
    render(<Input label="Họ tên" required />);
    const input = screen.getByLabelText(/Họ tên/);
    expect(input.className).toContain("focus-visible:ring-2");
    expect(input.className).not.toMatch(/(^|\s)focus:/);
    expect(input).toBeRequired();
  });

  it("works without a label or id and still forwards props", () => {
    render(<Input placeholder="Tìm" error="Lỗi" />);
    const input = screen.getByPlaceholderText("Tìm");
    expect(input).toHaveAccessibleDescription("Lỗi");
  });
});
