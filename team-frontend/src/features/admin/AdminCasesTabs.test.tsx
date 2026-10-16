import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { AdminCasesTabs } from "./AdminCasesTabs";

// Contract under test (implementer creates src/features/admin/AdminCasesTabs.tsx):
//   <AdminCasesTabs active={"reports" | "queue"} /> renders a tab strip of
//   plain links built from a data list (so another slice can append a tab):
//   "Báo cáo" -> /admin/cases?tab=reports, "Hàng chờ duyệt" -> /admin/cases?tab=queue.
//   The active tab link carries aria-current.

afterEach(cleanup);

describe("AdminCasesTabs", () => {
  it("renders both tabs as links and marks the active one (Moderation UI: Default tab is reports with status OPEN)", () => {
    render(<AdminCasesTabs active="reports" />);
    const reports = screen.getByRole("link", { name: "Báo cáo" });
    const queue = screen.getByRole("link", { name: "Hàng chờ duyệt" });
    expect(reports).toHaveAttribute("href", "/admin/cases?tab=reports");
    expect(queue).toHaveAttribute("href", "/admin/cases?tab=queue");
    expect(reports).toHaveAttribute("aria-current");
    expect(queue).not.toHaveAttribute("aria-current");
  });

  it("marks the queue tab when it is active (Moderation UI: Unknown tab or status falls back)", () => {
    render(<AdminCasesTabs active="queue" />);
    expect(
      screen.getByRole("link", { name: "Hàng chờ duyệt" }),
    ).toHaveAttribute("aria-current");
    expect(screen.getByRole("link", { name: "Báo cáo" })).not.toHaveAttribute(
      "aria-current",
    );
  });
});
