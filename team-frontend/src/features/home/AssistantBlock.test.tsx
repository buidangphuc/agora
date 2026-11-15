import { render } from "@testing-library/react";
import type { AnchorHTMLAttributes, ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { loadFeed } = vi.hoisted(() => ({ loadFeed: vi.fn() }));
vi.mock("./data", () => ({ loadFeed }));
vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: AnchorHTMLAttributes<HTMLAnchorElement> & {
    href: string;
    children?: ReactNode;
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

import { AssistantBlock } from "./AssistantBlock";

beforeEach(() => {
  vi.clearAllMocks();
  loadFeed.mockResolvedValue({ items: [] });
});

describe("AssistantBlock", () => {
  // Scenario: The home entry points to the assistant page
  it("links to the assistant page", async () => {
    const { container } = render(await AssistantBlock());
    expect(container.querySelector('a[href="/assistant"]')).not.toBeNull();
  });

  // Scenario: No invented score is rendered
  it("renders no match score", async () => {
    loadFeed.mockResolvedValue({
      items: [
        {
          id: "lst_1",
          title: "Tai nghe",
          price: 100,
          currency: "VND",
          imageUrl: "",
        },
      ],
    });
    const { container } = render(await AssistantBlock());
    expect(container.innerHTML).not.toContain("%");
    expect(container.textContent).not.toMatch(/Phù hợp \d+%/);
  });

  // Scenario: The entry does not load the feed
  it("does not depend on the feed", async () => {
    loadFeed.mockRejectedValue(new Error("feed down"));
    const { container } = render(await AssistantBlock());
    expect(container.querySelector('a[href="/assistant"]')).not.toBeNull();
    expect(loadFeed).not.toHaveBeenCalled();
  });
});
