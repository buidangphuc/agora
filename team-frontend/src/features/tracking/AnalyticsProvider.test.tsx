import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ConsentBanner } from "@/features/consent/ConsentBanner";

import { AnalyticsProvider } from "./AnalyticsProvider";

// Contract for the banner -> provider notification (design decided by the test author):
// ConsentBanner dispatches window CustomEvent "agora:consent:change" after writing the
// cookie, detail = { analytics, marketing, personalization } (the saved choice).
const CHANGE_EVENT = "agora:consent:change";

const gtmScripts = () =>
  Array.from(document.querySelectorAll("script")).filter((s) =>
    s.src.includes("googletagmanager.com"),
  );
const consentEntries = () =>
  ((window.dataLayer ?? []) as unknown[])
    .map((e) =>
      e && typeof e === "object" && "length" in e
        ? Array.from(e as ArrayLike<unknown>)
        : e,
    )
    .filter(
      (e): e is [string, string, Record<string, string>] =>
        Array.isArray(e) && e[0] === "consent",
    );
const setCookie = (v: string) => {
  document.cookie = `agora_consent=${v}; Path=/`;
};
const clearCookie = () => {
  document.cookie = "agora_consent=; Path=/; Max-Age=0";
};

describe("AnalyticsProvider GTM gating", () => {
  beforeEach(() => {
    document.head.innerHTML = "";
    delete (window as { dataLayer?: unknown }).dataLayer;
    clearCookie();
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    cleanup();
    clearCookie();
    vi.unstubAllEnvs();
  });

  it("[Empty GTM id keeps GTM off even with consent] a=1 cookie but empty id adds no script", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "");
    setCookie("v=1&a=1&m=1&p=1&ts=1760000000");
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[Rejected analytics never loads GTM] no cookie: mounting adds no script", () => {
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[Rejected analytics never loads GTM] a=0 cookie: no script", () => {
    setCookie("v=1&a=0&m=1&p=1&ts=1760000000");
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[Returning consented buyer loads GTM once] a=1 cookie: default then update, 1 script; remount keeps 1", () => {
    setCookie("v=1&a=1&m=0&p=0&ts=1760000000");
    const { unmount } = render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(consentEntries().map((e) => e[1])).toEqual(["default", "update"]);
    expect(gtmScripts().filter((s) => s.src.includes("GTM-TEST"))).toHaveLength(
      1,
    );
    unmount();
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    expect(gtmScripts()).toHaveLength(1);
  });

  it("[Accept analytics loads GTM and updates consent] clicking Accept all in the banner loads GTM", async () => {
    render(
      <AnalyticsProvider>
        <ConsentBanner />
      </AnalyticsProvider>,
    );
    expect(gtmScripts()).toHaveLength(0);
    await userEvent.click(screen.getByRole("button", { name: "Accept all" }));
    const kinds = consentEntries().map((e) => e[1]);
    expect(kinds).toEqual(["default", "update"]);
    expect(consentEntries()[1][2]).toMatchObject({
      analytics_storage: "granted",
      ad_storage: "granted",
      personalization_storage: "granted",
    });
    expect(gtmScripts()).toHaveLength(1);
  });

  it("[Marketing and personalization map to signals] change event detail analytics+marketing only", () => {
    render(<AnalyticsProvider>x</AnalyticsProvider>);
    act(() => {
      window.dispatchEvent(
        new CustomEvent(CHANGE_EVENT, {
          detail: { analytics: true, marketing: true, personalization: false },
        }),
      );
    });
    const update = consentEntries().find((e) => e[1] === "update");
    expect(update?.[2]).toMatchObject({
      analytics_storage: "granted",
      ad_user_data: "granted",
      personalization_storage: "denied",
    });
    expect(gtmScripts()).toHaveLength(1);
  });

  it("[Rejected analytics never loads GTM] Reject all in the banner adds no script", async () => {
    render(
      <AnalyticsProvider>
        <ConsentBanner />
      </AnalyticsProvider>,
    );
    await userEvent.click(screen.getByRole("button", { name: "Reject all" }));
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[Revoking analytics stops pushes] consented buyer saves a=0 via banner: update analytics_storage=denied pushed", async () => {
    setCookie("v=1&a=1&m=1&p=1&ts=1760000000");
    render(
      <AnalyticsProvider>
        <ConsentBanner />
      </AnalyticsProvider>,
    );
    act(() => {
      window.dispatchEvent(new CustomEvent("agora:consent:open"));
    });
    await userEvent.click(screen.getByRole("button", { name: "Reject all" }));
    const entries = consentEntries();
    expect(entries[entries.length - 1][1]).toBe("update");
    expect(entries[entries.length - 1][2].analytics_storage).toBe("denied");
    expect(document.cookie).toContain("a=0");
  });
});
