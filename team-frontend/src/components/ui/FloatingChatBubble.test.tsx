import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { FloatingChatBubble } from "./FloatingChatBubble";

vi.mock("next/navigation", () => ({ usePathname: () => "/listing/l1" }));

describe("FloatingChatBubble", () => {
  it("moves above the PDP buy bar below lg, so it does not cover the bar", () => {
    render(<FloatingChatBubble />);
    const wrapper = screen.getByRole("link", {
      name: "Mở hộp thư tin nhắn và hỗ trợ",
    }).parentElement;
    expect(wrapper?.className).toContain(
      "[body:has([data-testid=buy-bar])_&]:bottom-24",
    );
    expect(wrapper?.className).toContain(
      "lg:[body:has([data-testid=buy-bar])_&]:bottom-6",
    );
  });
});
