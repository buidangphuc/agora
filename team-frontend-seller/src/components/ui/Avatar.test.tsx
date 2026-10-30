import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Avatar, initials } from "./Avatar";

describe("Avatar", () => {
  it("derives initials", () => {
    expect(initials("Nguyễn Văn A")).toBe("NA");
    expect(initials("an")).toBe("A");
    expect(initials("")).toBe("?");
    expect(initials(undefined)).toBe("?");
  });

  it("without a src shows initials and is named", () => {
    render(<Avatar name="Trần Bình" />);
    expect(screen.getByRole("img", { name: "Trần Bình" })).toHaveTextContent(
      "TB",
    );
  });

  it("with a src shows the picture and falls back to initials on error", () => {
    render(<Avatar name="Lê Cường" src="/me.jpg" />);
    const pic = screen.getByAltText("Lê Cường");
    expect(pic).toHaveAttribute("src", "/me.jpg");
    fireEvent.error(pic);
    expect(screen.queryByAltText("Lê Cường")).toBeNull();
    expect(screen.getByText("LC")).toBeInTheDocument();
  });

  it("supports size and shape", () => {
    const { container } = render(<Avatar name="A" size="xl" shape="square" />);
    expect(container.firstChild).toHaveClass("h-20", "rounded-lg");
  });
});
