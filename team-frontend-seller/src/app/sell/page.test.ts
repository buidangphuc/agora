import { redirect } from "next/navigation";
import { describe, expect, it, vi } from "vitest";

import SellPage from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn() }));

describe("Seller onboarding UI: /sell keeps redirecting", () => {
  it("sell redirects to seller new", () => {
    SellPage();
    expect(redirect).toHaveBeenCalledWith("/seller/new");
  });
});
