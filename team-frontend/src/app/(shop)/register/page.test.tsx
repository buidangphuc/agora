import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@/features/auth/RegisterForm", () => ({
  RegisterForm: ({ returnUrl }: { returnUrl?: string }) => (
    <div data-testid="form" data-return-url={returnUrl ?? ""} />
  ),
}));

import Page from "./page";

async function renderPage(sp: Record<string, string | string[] | undefined>) {
  render(await Page({ searchParams: Promise.resolve(sp) }));
  return screen.getByTestId("form").getAttribute("data-return-url");
}

describe("register page returnUrl (frontend-login-return)", () => {
  it("passes a safe searchParams.returnUrl to the form", async () => {
    expect(await renderPage({ returnUrl: "/listing/abc" })).toBe(
      "/listing/abc",
    );
  });

  it("passes no returnUrl when absent", async () => {
    expect(await renderPage({})).toBe("");
  });

  it("does not pass an unsafe returnUrl to the form", async () => {
    const v = await renderPage({ returnUrl: "https://evil.example" });
    expect(v).not.toContain("evil");
  });
});
