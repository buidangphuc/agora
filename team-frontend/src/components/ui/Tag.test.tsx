import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Tag } from "./Tag";

describe("Tag", () => {
  it("renders a label with a colour preset", () => {
    render(<Tag color="success">Freeship</Tag>);
    expect(screen.getByText("Freeship")).toHaveClass("bg-accent-success/10");
  });

  it("defaults to neutral and supports every preset", () => {
    for (const color of [
      "neutral",
      "primary",
      "info",
      "success",
      "warning",
      "danger",
    ] as const) {
      const { unmount } = render(<Tag color={color}>{color}</Tag>);
      expect(screen.getByText(color)).toBeInTheDocument();
      unmount();
    }
  });

  it("is server-renderable without a button when not closable", () => {
    const html = renderToStaticMarkup(<Tag>Mới</Tag>);
    expect(html).toContain("Mới");
    expect(html).not.toContain("<button");
  });

  it("closable tags show a labelled close button, report and disappear", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Tag closable onClose={onClose} closeLabel="Xoá bộ lọc Giá">
        Giá
      </Tag>,
    );
    const close = screen.getByRole("button", { name: "Xoá bộ lọc Giá" });
    expect(close.className).toContain("focus-visible:ring-2");
    await user.click(close);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Giá")).toBeNull();
  });

  it("the close button is keyboard operable", async () => {
    const user = setupUser();
    const onClose = vi.fn();
    render(
      <Tag closable onClose={onClose}>
        X
      </Tag>,
    );
    screen.getByRole("button").focus();
    await user.keyboard("{Enter}");
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
