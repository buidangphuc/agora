import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ChatWithSellerButton } from "./ChatWithSellerButton";
import { getOrCreateThreadAction } from "./actions";

const push = vi.hoisted(() => vi.fn());
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("./actions", () => ({ getOrCreateThreadAction: vi.fn() }));

beforeEach(() => vi.clearAllMocks());

describe("ChatWithSellerButton", () => {
  it("[Every guard uses the helper] guest click navigates to an encoded login path", () => {
    render(
      <ChatWithSellerButton sellerId="s1" listingId="abc" loggedIn={false} />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(push).toHaveBeenCalledWith("/login?returnUrl=%2Flisting%2Fabc");
    expect(getOrCreateThreadAction).not.toHaveBeenCalled();
  });

  it("[Every guard uses the helper] guest click encodes a listing id with reserved characters", () => {
    render(
      <ChatWithSellerButton sellerId="s1" listingId="a b" loggedIn={false} />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(push).toHaveBeenCalledWith(
      `/login?returnUrl=${encodeURIComponent("/listing/a b")}`,
    );
  });
});
