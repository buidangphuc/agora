import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Badge } from "./Badge";

describe("Badge", () => {
  it("keeps every existing variant working", () => {
    for (const variant of [
      "primary",
      "mall",
      "success",
      "warning",
      "danger",
      "neutral",
      "discount",
    ] as const) {
      const { unmount } = render(<Badge variant={variant}>{variant}</Badge>);
      expect(screen.getByText(variant)).toBeInTheDocument();
      unmount();
    }
  });

  it("mall and discount are solid fills", () => {
    render(
      <>
        <Badge variant="mall">Mall</Badge>
        <Badge variant="discount">-10%</Badge>
      </>,
    );
    expect(screen.getByText("Mall")).toHaveClass(
      "bg-action-danger",
      "text-text-inverse",
    );
    expect(screen.getByText("-10%")).toHaveClass(
      "bg-text-warning",
      "text-text-inverse",
    );
  });

  it("supports pill, size and passes through attributes", () => {
    render(
      <Badge pill size="md" data-x="1">
        3
      </Badge>,
    );
    const el = screen.getByText("3");
    expect(el).toHaveClass("rounded-full");
    expect(el).toHaveAttribute("data-x", "1");
  });
});
