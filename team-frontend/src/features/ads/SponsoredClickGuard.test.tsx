import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SponsoredClickGuard } from "./SponsoredClickGuard";

const sendBeacon = vi.fn<(url: string, data?: unknown) => boolean>();
const fetchMock = vi.fn();

async function bodyOf(data: unknown): Promise<unknown> {
  if (typeof data === "string") return JSON.parse(data);
  const blob = data as Blob;
  const text = await new Promise<string>((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result));
    r.onerror = () => reject(r.error);
    r.readAsText(blob);
  });
  return JSON.parse(text);
}

function renderGuard() {
  return render(
    <SponsoredClickGuard>
      <div data-testid="sponsored-slot" data-impression-token="tok-1">
        <a href="/listings/l1" data-testid="sponsored-link">
          sponsored
        </a>
      </div>
      <div data-testid="search-results">
        <a href="/listings/l2" data-testid="organic-link">
          organic
        </a>
      </div>
    </SponsoredClickGuard>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  sendBeacon.mockReturnValue(true);
  Object.defineProperty(navigator, "sendBeacon", {
    configurable: true,
    value: sendBeacon,
  });
  vi.stubGlobal("fetch", fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("SponsoredClickGuard", () => {
  it("sends one beacon and keeps default navigation (Click beacon fires once per gesture and does not prevent navigation)", async () => {
    renderGuard();
    const link = screen.getByTestId("sponsored-link");

    const notPrevented = fireEvent.click(link);

    expect(notPrevented).toBe(true);
    expect(sendBeacon).toHaveBeenCalledTimes(1);
    const [url, data] = sendBeacon.mock.calls[0];
    expect(url).toBe("/api/ads/click");
    expect(await bodyOf(data)).toEqual({ token: "tok-1" });
  });

  it("sends one beacon for a middle click and keeps default navigation (Click beacon fires once per gesture and does not prevent navigation)", async () => {
    renderGuard();
    const link = screen.getByTestId("sponsored-link");

    const notPrevented = fireEvent(
      link,
      new MouseEvent("auxclick", {
        bubbles: true,
        cancelable: true,
        button: 1,
      }),
    );

    expect(notPrevented).toBe(true);
    expect(sendBeacon).toHaveBeenCalledTimes(1);
    expect(await bodyOf(sendBeacon.mock.calls[0][1])).toEqual({
      token: "tok-1",
    });
  });

  it("falls back to keepalive fetch when sendBeacon is missing (Click beacon fires once per gesture and does not prevent navigation)", async () => {
    Object.defineProperty(navigator, "sendBeacon", {
      configurable: true,
      value: undefined,
    });
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));
    renderGuard();

    const notPrevented = fireEvent.click(screen.getByTestId("sponsored-link"));

    expect(notPrevented).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/ads/click");
    expect(init.method).toBe("POST");
    expect(init.keepalive).toBe(true);
    expect(JSON.parse(String(init.body))).toEqual({ token: "tok-1" });
  });

  it("organic clicks send no beacon (Click on an organic card sends no beacon)", () => {
    renderGuard();

    const notPrevented = fireEvent.click(screen.getByTestId("organic-link"));
    fireEvent(
      screen.getByTestId("organic-link"),
      new MouseEvent("auxclick", {
        bubbles: true,
        cancelable: true,
        button: 1,
      }),
    );

    expect(notPrevented).toBe(true);
    expect(sendBeacon).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
