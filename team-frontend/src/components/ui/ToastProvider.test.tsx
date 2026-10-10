import { act, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ToastProvider, useToast } from "./ToastProvider";

function Trigger({ kind }: { kind: "error" | "success" | "info" }) {
  const toast = useToast();
  return (
    <button type="button" onClick={() => toast[kind](`${kind} message`)}>
      fire
    </button>
  );
}

describe("ToastProvider roles", () => {
  it("announces an error toast as an alert", () => {
    render(
      <ToastProvider>
        <Trigger kind="error" />
      </ToastProvider>,
    );
    act(() => screen.getByText("fire").click());
    expect(screen.getByRole("alert")).toHaveTextContent("error message");
  });

  it("announces success and info toasts as status, not alert", () => {
    render(
      <ToastProvider>
        <Trigger kind="success" />
      </ToastProvider>,
    );
    act(() => screen.getByText("fire").click());
    expect(screen.getByRole("status")).toHaveTextContent("success message");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
