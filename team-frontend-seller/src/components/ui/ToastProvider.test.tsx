import { act, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ToastProvider, toast, useToast } from "./ToastProvider";

function Trigger({ kind }: { kind: "error" | "success" | "info" }) {
  const toaster = useToast();
  return (
    <button type="button" onClick={() => toaster[kind](`${kind} message`)}>
      fire
    </button>
  );
}

function statusesWith(text: string) {
  return screen
    .queryAllByRole("status")
    .filter((el) => el.textContent?.includes(text));
}

describe("ToastProvider roles", () => {
  it("announces an error toast as an alert", async () => {
    render(
      <ToastProvider>
        <Trigger kind="error" />
      </ToastProvider>,
    );
    act(() => screen.getByText("fire").click());
    expect(await screen.findByRole("alert")).toHaveTextContent("error message");
  });

  it("announces success and info toasts as one status, not alert", async () => {
    render(
      <ToastProvider>
        <Trigger kind="success" />
      </ToastProvider>,
    );
    act(() => screen.getByText("fire").click());
    await screen.findByText("success message");
    expect(statusesWith("success message")).toHaveLength(1);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows toasts sent through the global helper", async () => {
    render(
      <ToastProvider>
        <p>app</p>
      </ToastProvider>,
    );
    act(() => toast.error("from outside"));
    expect(await screen.findByRole("alert")).toHaveTextContent("from outside");
  });

  it("closes a toast from its dismiss button", async () => {
    render(
      <ToastProvider>
        <Trigger kind="info" />
      </ToastProvider>,
    );
    act(() => screen.getByText("fire").click());
    await screen.findByText("info message");
    act(() => screen.getByRole("button", { name: "Đóng" }).click());
    expect(screen.queryByText("info message")).toBeNull();
  });
});
