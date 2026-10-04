import { render, screen } from "@testing-library/react";
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setupUser } from "@/test/user";

import { OrderDetailTabs } from "./OrderDetailTabs";
import { parseDetailTab } from "./detailTab";

afterEach(() => vi.restoreAllMocks());

describe("OrderDetailTabs", () => {
  it("starts on the tab from the URL and keeps both panels mounted", () => {
    render(
      <OrderDetailTabs
        initialTab="returns"
        timeline={<p>timeline body</p>}
        returns={<p>returns body</p>}
      />,
    );
    expect(
      screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }),
    ).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("returns body")).toBeVisible();
    expect(
      screen.getByText("timeline body", { selector: "p" }),
    ).not.toBeVisible();
  });

  it("writes the selected tab to ?tab= and drops it for the default", async () => {
    const replace = vi.spyOn(window.history, "replaceState");
    render(
      <OrderDetailTabs
        initialTab="timeline"
        timeline={<p>timeline body</p>}
        returns={<p>returns body</p>}
      />,
    );
    const user = setupUser();
    await user.click(screen.getByRole("tab", { name: "Trả hàng / Hoàn tiền" }));
    expect(replace).toHaveBeenLastCalledWith(
      null,
      "",
      expect.stringContaining("tab=returns"),
    );
    expect(screen.getByText("returns body")).toBeVisible();

    await user.click(screen.getByRole("tab", { name: "Hành trình" }));
    expect(replace).toHaveBeenLastCalledWith(
      null,
      "",
      expect.not.stringContaining("tab="),
    );
  });

  it("parses unknown tab values as timeline", () => {
    expect(parseDetailTab("returns")).toBe("returns");
    expect(parseDetailTab("x")).toBe("timeline");
    expect(parseDetailTab(undefined)).toBe("timeline");
  });
});
