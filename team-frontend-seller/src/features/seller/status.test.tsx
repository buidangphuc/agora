import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ListingStatusTag } from "./status";

describe("ListingStatusTag", () => {
  // Scenario: Moderation UI: New status tags
  it("maps the new listing statuses", () => {
    const { rerender } = render(<ListingStatusTag status="pending_review" />);
    expect(screen.getByText("Chờ duyệt")).toBeTruthy();
    rerender(<ListingStatusTag status="banned" />);
    expect(screen.getByText("Bị khóa")).toBeTruthy();
    rerender(<ListingStatusTag status="held" />);
    expect(screen.getByText("Tạm ẩn")).toBeTruthy();
    // existing tags are untouched
    rerender(<ListingStatusTag status="published" />);
    expect(screen.getByText("Đang bán")).toBeTruthy();
    rerender(<ListingStatusTag status="draft" />);
    expect(screen.getByText("Bản nháp")).toBeTruthy();
    rerender(<ListingStatusTag status="rejected" />);
    expect(screen.getByText("Bị từ chối")).toBeTruthy();
  });

  // Scenario: Moderation UI: Unknown status stays neutral
  it("falls back for an unknown status", () => {
    render(<ListingStatusTag status="something_new" />);
    expect(screen.getByText("Chưa xác định")).toBeTruthy();
  });
});
