import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const startBuyerHandoff = vi.hoisted(() => vi.fn());
vi.mock("./actions", () => ({ startBuyerHandoff }));

import { BuyerStorefrontLink } from "./BuyerStorefrontLink";

const assign = vi.fn();
const LOGIN = "http://localhost:3000/login?next=%2F";
const HANDOFF = "http://localhost:3000/auth/handoff?code=CODE123&next=%2F";

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("location", { assign, href: "http://localhost:3001/" });
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("seller BuyerStorefrontLink", () => {
  it("[Code minted on click, never at render] renders the login URL and never calls startHandoff before a click", () => {
    const { container } = render(
      <BuyerStorefrontLink>Về trang mua sắm</BuyerStorefrontLink>,
    );
    expect(startBuyerHandoff).not.toHaveBeenCalled();
    expect(container.innerHTML).not.toContain("code=");
    expect(
      screen.getByRole("link", { name: "Về trang mua sắm" }),
    ).toHaveAttribute("href", LOGIN);
  });

  it("[Seller to buyer signed in] seller buyer link calls startHandoff on click only and navigates to the minted URL", async () => {
    startBuyerHandoff.mockResolvedValue(HANDOFF);
    render(<BuyerStorefrontLink>Về trang mua sắm</BuyerStorefrontLink>);
    fireEvent.click(screen.getByRole("link", { name: "Về trang mua sắm" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(HANDOFF));
    expect(startBuyerHandoff).toHaveBeenCalledTimes(1);
    expect(startBuyerHandoff).toHaveBeenCalledWith("/");
  });

  it("[Mint failure still navigates] mint failure falls back to login link", async () => {
    startBuyerHandoff.mockRejectedValue(new Error("mint failed"));
    render(<BuyerStorefrontLink>Về trang mua sắm</BuyerStorefrontLink>);
    fireEvent.click(screen.getByRole("link", { name: "Về trang mua sắm" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(LOGIN));
  });

  it("[Cap reached still navigates] cap reached falls back to login link", async () => {
    startBuyerHandoff.mockResolvedValue(LOGIN);
    render(<BuyerStorefrontLink>Về trang mua sắm</BuyerStorefrontLink>);
    fireEvent.click(screen.getByRole("link", { name: "Về trang mua sắm" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(LOGIN));
    expect(assign).toHaveBeenCalledTimes(1);
  });
});
