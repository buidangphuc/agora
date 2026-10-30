import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Card, CardContent, CardHeader, CardTitle } from "./Card";

describe("Card", () => {
  it("renders its parts", () => {
    render(
      <Card>
        <CardHeader>
          <CardTitle>Tiêu đề</CardTitle>
        </CardHeader>
        <CardContent>Nội dung</CardContent>
      </Card>,
    );
    expect(
      screen.getByRole("heading", { name: "Tiêu đề" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Nội dung")).toBeInTheDocument();
  });

  it("loading shows a skeleton instead of the content and is busy", () => {
    const { container } = render(
      <Card loading>
        <CardContent>Nội dung</CardContent>
      </Card>,
    );
    expect(screen.queryByText("Nội dung")).toBeNull();
    expect(container.firstChild).toHaveAttribute("aria-busy", "true");
    expect(container.querySelectorAll(".animate-pulse").length).toBe(3);
  });

  it("is flush by default and padded on request", () => {
    const flush = render(<Card>x</Card>);
    expect((flush.container.firstChild as HTMLElement).className).not.toContain(
      "p-6",
    );
    const padded = render(<Card padded>x</Card>);
    expect((padded.container.firstChild as HTMLElement).className).toContain(
      "p-6",
    );
  });

  it("hoverable adds the hover treatment and uses alias tokens", () => {
    const { container } = render(<Card hoverable>x</Card>);
    const cls = (container.firstChild as HTMLElement).className;
    expect(cls).toContain("hover:border-border-strong");
    expect(cls).toContain("bg-surface-card");
    expect(cls).toContain("border-border-subtle");
  });
});
