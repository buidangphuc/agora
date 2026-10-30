import { redirect } from "next/navigation";
import { describe, expect, it, vi } from "vitest";

import SellPage from "./page";

vi.mock("next/navigation", () => ({ redirect: vi.fn() }));

describe("/sell", () => {
  it("is a server redirect to the listing studio", () => {
    SellPage();
    expect(redirect).toHaveBeenCalledWith("/seller/new");
  });
});
