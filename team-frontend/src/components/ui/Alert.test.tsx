import { render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";
import { Alert } from "./Alert";

describe("Alert", () => {
  it("error uses role=alert, other types are polite status", () => {
    const { rerender } = render(<Alert type="error" title="Lỗi" />);
    expect(screen.getByRole("alert")).toHaveTextContent("Lỗi");
    for (const type of ["info", "success", "warning"] as const) {
      rerender(<Alert type={type} title={type} />);
      expect(screen.queryByRole("alert")).toBeNull();
      expect(screen.getByRole("status")).toHaveTextContent(type);
    }
  });

  it("renders title, description and an action", () => {
    render(
      <Alert
        type="warning"
        title="Sắp hết hàng"
        description="Còn 2 sản phẩm"
        action={<a href="/x">Xem</a>}
      />,
    );
    expect(screen.getByText("Còn 2 sản phẩm")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem" })).toBeInTheDocument();
  });

  it("is server-renderable without a close button by default", () => {
    const html = renderToStaticMarkup(<Alert type="info" title="Hi" />);
    expect(html).toContain("Hi");
    expect(html).not.toContain("<button");
  });

  it("closable alerts close and report it", async () => {
    const onClose = vi.fn();
    const user = setupUser();
    render(<Alert type="success" title="Xong" closable onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Đóng" }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Xong")).toBeNull();
  });
});
