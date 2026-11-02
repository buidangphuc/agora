import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const startSellerHandoff = vi.hoisted(() => vi.fn());
vi.mock("./actions", () => ({ startSellerHandoff }));

import { SellerChannelLink } from "./SellerChannelLink";

const assign = vi.fn();
const LOGIN = "http://localhost:3001/login?next=%2Fseller";
const HANDOFF =
  "http://localhost:3001/auth/handoff?code=CODE123&next=%2Fseller";

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("location", { assign, href: "http://localhost:3000/" });
});
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("buyer SellerChannelLink", () => {
  it("[Code minted on click, never at render] renders the login URL and never calls startHandoff before a click", () => {
    const { container } = render(
      <SellerChannelLink>Kênh người bán</SellerChannelLink>,
    );
    expect(startSellerHandoff).not.toHaveBeenCalled();
    expect(container.innerHTML).not.toContain("code=");
    expect(
      screen.getByRole("link", { name: "Kênh người bán" }),
    ).toHaveAttribute("href", LOGIN);
  });

  it("[Buyer to seller signed in] buyer seller link calls startHandoff on click only and navigates to the minted URL", async () => {
    startSellerHandoff.mockResolvedValue(HANDOFF);
    render(<SellerChannelLink>Kênh người bán</SellerChannelLink>);
    fireEvent.click(screen.getByRole("link", { name: "Kênh người bán" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(HANDOFF));
    expect(startSellerHandoff).toHaveBeenCalledTimes(1);
    expect(startSellerHandoff).toHaveBeenCalledWith("/seller");
  });

  it("[Mint failure still navigates] mint failure falls back to login link", async () => {
    startSellerHandoff.mockRejectedValue(new Error("mint failed"));
    render(<SellerChannelLink>Kênh người bán</SellerChannelLink>);
    fireEvent.click(screen.getByRole("link", { name: "Kênh người bán" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(LOGIN));
  });

  it("[Cap reached still navigates] cap reached falls back to login link", async () => {
    startSellerHandoff.mockResolvedValue(LOGIN);
    render(<SellerChannelLink>Kênh người bán</SellerChannelLink>);
    fireEvent.click(screen.getByRole("link", { name: "Kênh người bán" }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith(LOGIN));
    expect(assign).toHaveBeenCalledTimes(1);
  });
});
