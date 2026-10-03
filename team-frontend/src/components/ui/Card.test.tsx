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
    expect(container.querySelector('[data-variant="text"]')).not.toBeNull();
  });

  it("hoverable adds the hover treatment and uses alias tokens", () => {
    const { container } = render(<Card hoverable>x</Card>);
    const cls = (container.firstChild as HTMLElement).className;
    expect(cls).toContain("hover:-translate-y-0.5");
    expect(cls).toContain("bg-surface-card");
    expect(cls).toContain("border-border-subtle");
  });
});
