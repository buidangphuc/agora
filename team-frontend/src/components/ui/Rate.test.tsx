import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Rate } from "./Rate";

describe("Rate", () => {
  it("read-only is a static image with a text summary and no inputs", () => {
    const { container } = render(<Rate readOnly value={4.5} />);
    expect(
      screen.getByRole("img", { name: "4.5 trên 5 sao" }),
    ).toBeInTheDocument();
    expect(container.querySelectorAll("input")).toHaveLength(0);
    // 4 full stars + 1 half
    expect(container.querySelectorAll(".w-full")).toHaveLength(4);
    expect(container.querySelectorAll(".w-1\\/2")).toHaveLength(1);
  });

  it("read-only renders on the server", () => {
    const html = renderToStaticMarkup(<Rate readOnly value={3} />);
    expect(html).toContain("3 trên 5 sao");
  });

  it("input mode is a labelled radio group; click selects and reports", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Rate onChange={onChange} />);
    expect(screen.getByRole("group", { name: "Đánh giá" })).toBeInTheDocument();
    await user.click(screen.getByRole("radio", { name: "4 sao" }));
    expect(onChange).toHaveBeenCalledWith(4);
    expect(screen.getByRole("radio", { name: "4 sao" })).toBeChecked();
  });

  it("is keyboard operable with arrow keys", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Rate defaultValue={2} onChange={onChange} />);
    screen.getByRole("radio", { name: "2 sao" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith(3);
    expect(screen.getByRole("radio", { name: "3 sao" })).toBeChecked();
  });

  it("shows a keyboard focus ring on the star", () => {
    const { container } = render(<Rate />);
    expect(container.innerHTML).toContain("peer-focus-visible:ring-2");
  });

  it("disabled is announced and inert", async () => {
    const user = setupUser();
    const onChange = vi.fn();
    render(<Rate disabled onChange={onChange} />);
    expect(screen.getByRole("group")).toHaveAttribute("aria-disabled", "true");
    await user.click(screen.getByRole("radio", { name: "3 sao" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("supports controlled value", () => {
    render(<Rate value={2} />);
    expect(screen.getByRole("radio", { name: "2 sao" })).toBeChecked();
  });
});
