import { render, screen, within } from "@testing-library/react";
import { notFound } from "next/navigation";
import { afterEach, describe, expect, it, vi } from "vitest";

import UiCataloguePage from "./page";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.mocked(notFound).mockClear();
});

describe("/dev/ui catalogue", () => {
  it("smoke: renders every section with its states outside production", () => {
    vi.stubEnv("NODE_ENV", "development");
    render(<UiCataloguePage />);
    expect(notFound).not.toHaveBeenCalled();

    for (const id of [
      "button",
      "data-entry",
      "navigation",
      "data-display",
      "feedback",
    ]) {
      expect(screen.getByTestId(`ui-section-${id}`)).toBeInTheDocument();
    }

    // Button states
    expect(screen.getByTestId("ui-button-loading")).toHaveAttribute(
      "aria-busy",
      "true",
    );
    expect(screen.getByTestId("ui-button-disabled")).toBeDisabled();

    // Data states: empty, loading, error
    expect(screen.getByText("Chưa có sản phẩm")).toBeInTheDocument();
    expect(
      screen.getAllByText("Không tải được danh sách").length,
    ).toBeGreaterThan(0);
    expect(
      document.querySelectorAll("[aria-busy='true']").length,
    ).toBeGreaterThan(3);

    // Navigation: link tabs and pagination work without client state
    const pagination = screen.getAllByRole("navigation", {
      name: "Phân trang",
    })[0];
    expect(
      within(pagination).getAllByRole("link", { name: "Trang 2" })[0],
    ).toHaveAttribute("aria-current", "page");

    // Data entry / feedback
    expect(screen.getByTestId("ui-quantity-value")).toHaveTextContent("3");
    expect(screen.getAllByRole("alert").length).toBeGreaterThan(0);
    expect(screen.getByTestId("ui-modal-open")).toBeInTheDocument();
  });

  it("is a 404 in production", () => {
    vi.stubEnv("NODE_ENV", "production");
    render(<UiCataloguePage />);
    expect(notFound).toHaveBeenCalledTimes(1);
  });
});

describe("/dev/ui search params", () => {
  it("drive the link tabs and the pagination so back/forward restore them", () => {
    vi.stubEnv("NODE_ENV", "development");
    render(<UiCataloguePage searchParams={{ status: "done", page: "4" }} />);
    const linkTabs = screen.getByRole("navigation", { name: "Tabs" });
    expect(
      within(linkTabs).getByRole("link", { name: "Hoàn tất" }),
    ).toHaveAttribute("aria-current", "page");
    const pagination = screen.getAllByRole("navigation", {
      name: "Phân trang",
    })[0];
    expect(
      within(pagination).getByRole("link", { name: "Trang 4" }),
    ).toHaveAttribute("aria-current", "page");
  });
});
