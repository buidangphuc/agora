import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const { trackEcommerce, subscribeAlertAction, unsubscribeAlertAction, toast } =
  vi.hoisted(() => ({
    trackEcommerce: vi.fn(),
    subscribeAlertAction: vi.fn(),
    unsubscribeAlertAction: vi.fn(),
    toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  }));
vi.mock("@/lib/analytics", () => ({ trackEcommerce }));
vi.mock("@/features/notification/actions", () => ({
  subscribeAlertAction,
  unsubscribeAlertAction,
}));
vi.mock("@/components/ui/ToastProvider", () => ({ useToast: () => toast }));

import { AlertToggle } from "./AlertToggle";

const toggle = (kind: string): HTMLElement => {
  const el = screen
    .getAllByTestId("alert-toggle")
    .find((x) => x.getAttribute("data-type") === kind);
  if (!el) throw new Error(`no alert toggle of kind ${kind}`);
  return el;
};

beforeEach(() => vi.clearAllMocks());

describe("AlertToggle loop events", () => {
  it("tracks alert_subscribe with the type", async () => {
    subscribeAlertAction.mockResolvedValue({ ok: true, subscriptionId: "s1" });
    render(<AlertToggle listingId="L1" initial={{}} loggedIn={true} />);
    fireEvent.click(toggle("price_drop"));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("alert_subscribe", {
      items: [{ itemId: "L1" }],
      properties: { alert_type: "price_drop" },
    });
  });

  it("tracks alert_subscribe with the type: back_in_stock", async () => {
    subscribeAlertAction.mockResolvedValue({ ok: true, subscriptionId: "s2" });
    render(<AlertToggle listingId="L1" initial={{}} loggedIn={true} />);
    fireEvent.click(toggle("back_in_stock"));
    await waitFor(() => expect(trackEcommerce).toHaveBeenCalledTimes(1));
    expect(trackEcommerce).toHaveBeenCalledWith("alert_subscribe", {
      items: [{ itemId: "L1" }],
      properties: { alert_type: "back_in_stock" },
    });
  });

  it("tracks nothing off failure or guest: unsubscribe", async () => {
    unsubscribeAlertAction.mockResolvedValue({ ok: true });
    render(
      <AlertToggle
        listingId="L1"
        initial={{ priceDropSubId: "s1" }}
        loggedIn={true}
      />,
    );
    fireEvent.click(toggle("price_drop"));
    await waitFor(() => expect(unsubscribeAlertAction).toHaveBeenCalled());
    await waitFor(() => expect(toast.info).toHaveBeenCalled());
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing off failure or guest: failed subscribe", async () => {
    subscribeAlertAction.mockResolvedValue({ ok: false, message: "x" });
    render(<AlertToggle listingId="L1" initial={{}} loggedIn={true} />);
    fireEvent.click(toggle("price_drop"));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(trackEcommerce).not.toHaveBeenCalled();
  });

  it("tracks nothing off failure or guest: guest click", () => {
    render(<AlertToggle listingId="L1" initial={{}} loggedIn={false} />);
    fireEvent.click(toggle("price_drop"));
    expect(subscribeAlertAction).not.toHaveBeenCalled();
    expect(trackEcommerce).not.toHaveBeenCalled();
  });
});
