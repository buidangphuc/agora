import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import axe from "axe-core";
import { afterEach, describe, expect, it, vi } from "vitest";
import InternalErrorBoundary from "../app/(internal)/error";
import ErrorBoundary from "../app/error";
import NotFound from "../app/not-found";

afterEach(cleanup);

async function violations(node: Element) {
  const result = await axe.run(node, {
    rules: { "color-contrast": { enabled: false }, region: { enabled: false } },
  });
  return result.violations.map((v) => `${v.id}: ${v.help}`);
}

describe("error boundaries", () => {
  it("root boundary has no axe violations and resets", async () => {
    const reset = vi.fn();
    const { baseElement } = render(
      <ErrorBoundary
        error={Object.assign(new Error("x"), { digest: "d1" })}
        reset={reset}
      />,
    );
    expect(screen.getByText("Reference: d1")).toBeTruthy();
    expect(await violations(baseElement)).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    expect(reset).toHaveBeenCalledOnce();
  });

  it("internal boundary has no axe violations", async () => {
    const { baseElement } = render(
      <InternalErrorBoundary error={new Error("x")} reset={() => {}} />,
    );
    expect(await violations(baseElement)).toEqual([]);
  });

  it("not-found has no axe violations", async () => {
    const { baseElement } = render(<NotFound />);
    expect(await violations(baseElement)).toEqual([]);
  });
});
