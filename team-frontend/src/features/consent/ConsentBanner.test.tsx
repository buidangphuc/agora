import { readFileSync } from "node:fs";
import { join } from "node:path";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readConsent } from "@/lib/consent/consent";

const ledger = vi.hoisted(() => ({ recordBannerConsent: vi.fn() }));
vi.mock("./actions", () => ({
  recordBannerConsent: ledger.recordBannerConsent,
}));

import { ConsentBanner, CookieSettingsLink } from "./ConsentBanner";

const NOW_MS = 1760000000_000;

function clearCookie() {
  document.cookie = "agora_consent=; Path=/; Max-Age=0";
}
function rawCookie(): string {
  const m = document.cookie.match(/(?:^|;\s*)agora_consent=([^;]*)/);
  return m ? m[1] : "";
}
function mountPage() {
  return render(
    <>
      <ConsentBanner />
      <footer>
        <CookieSettingsLink />
      </footer>
    </>,
  );
}

beforeEach(() => {
  ledger.recordBannerConsent.mockReset();
  ledger.recordBannerConsent.mockResolvedValue(undefined);
  clearCookie();
  vi.spyOn(Date, "now").mockReturnValue(NOW_MS);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  clearCookie();
});

describe("Scenario: First visit shows banner and contacts no Google host", () => {
  it("shows a dialog with analytics, marketing, personalization groups", async () => {
    mountPage();
    const dialog = await screen.findByRole("dialog");
    for (const g of [/Phân tích/i, /Tiếp thị/i, /Cá nhân hóa/i]) {
      expect(
        await screen.findByRole("checkbox", { name: g }),
      ).toBeInTheDocument();
    }
    expect(dialog).toHaveAccessibleName();
    for (const b of [/Chấp nhận tất cả/i, /Từ chối tất cả/i, /Lưu lựa chọn/i]) {
      expect(screen.getByRole("button", { name: b })).toBeInTheDocument();
    }
  });

  it("starts with every optional group unchecked (opt-in)", async () => {
    mountPage();
    for (const g of [/Phân tích/i, /Tiếp thị/i, /Cá nhân hóa/i]) {
      expect(
        await screen.findByRole("checkbox", { name: g }),
      ).not.toBeChecked();
    }
  });

  it("injects no googletagmanager/google-analytics script and no request", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
    mountPage();
    await screen.findByRole("dialog");
    expect(
      document.querySelectorAll(
        "script[src*='googletagmanager.com'], script[src*='google-analytics.com']",
      ),
    ).toHaveLength(0);
    expect(fetchSpy).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
  });

  it("does not write a cookie until the buyer chooses", async () => {
    mountPage();
    await screen.findByRole("dialog");
    expect(rawCookie()).toBe("");
  });
});

describe("Scenario: Accept all writes the cookie / Reject all / Granular choice (banner side)", () => {
  it("Accept all writes v=1&a=1&m=1&p=1 and removes the banner", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("button", { name: /Chấp nhận tất cả/i }),
    );
    expect(rawCookie()).toBe("v=1&a=1&m=1&p=1&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("Reject all writes v=1&a=0&m=0&p=0 and removes the banner", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("button", { name: /Từ chối tất cả/i }),
    );
    expect(rawCookie()).toBe("v=1&a=0&m=0&p=0&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("Save choices with only personalization writes a=0&m=0&p=1", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("checkbox", { name: /Cá nhân hóa/i }),
    );
    await user.click(screen.getByRole("button", { name: /Lưu lựa chọn/i }));
    expect(rawCookie()).toBe("v=1&a=0&m=0&p=1&ts=1760000000");
    expect(readConsent()).toMatchObject({
      analytics: false,
      marketing: false,
      personalization: true,
      version: 1,
    });
  });
});

describe("Scenario: Banner does not shift layout", () => {
  it("renders in a fixed-position layer (out of document flow)", async () => {
    mountPage();
    const dialog = await screen.findByRole("dialog");
    const layer = dialog.closest(".fixed") ?? dialog;
    const cls = `${layer.className} ${dialog.className}`;
    const inline = (layer as HTMLElement).style.position;
    expect(/(^|\s)fixed(\s|$)/.test(cls) || inline === "fixed").toBe(true);
  });

  it("is not wrapped in a static-flow container that contributes layout", async () => {
    const { container } = mountPage();
    await screen.findByRole("dialog");
    // the banner must not add a non-fixed sibling element before the footer
    const staticBlocks = Array.from(container.children).filter(
      (el) =>
        el.tagName !== "FOOTER" &&
        !el.closest(".fixed") &&
        !el.classList.contains("fixed"),
    );
    expect(staticBlocks).toHaveLength(0);
  });
});

describe("Scenario: Returning buyer sees no banner", () => {
  beforeEach(() => {
    document.cookie = "agora_consent=v=1&a=0&m=0&p=0&ts=1760000000; Path=/";
  });

  it("renders no dialog with a valid cookie", async () => {
    mountPage();
    await screen.findByText(/Cài đặt cookie/i);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("footer 'Cookie settings' link re-opens the banner", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByText(/Cài đặt cookie/i));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("re-opened banner reflects stored choices and saving updates the cookie", async () => {
    document.cookie = "agora_consent=v=1&a=0&m=0&p=1&ts=1760000000; Path=/";
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByText(/Cài đặt cookie/i));
    expect(
      await screen.findByRole("checkbox", { name: /Cá nhân hóa/i }),
    ).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: /Phân tích/i }));
    await user.click(screen.getByRole("button", { name: /Lưu lựa chọn/i }));
    expect(rawCookie()).toBe("v=1&a=1&m=0&p=1&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("an invalid cookie (unknown version) still shows the banner", async () => {
    document.cookie = "agora_consent=v=2&a=1&m=1&p=1&ts=1760000000; Path=/";
    mountPage();
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });
});

describe("Scenario: Banner is keyboard operable", () => {
  it("has role=dialog with an accessible name", async () => {
    mountPage();
    expect(await screen.findByRole("dialog")).toHaveAccessibleName();
  });

  it("every control is reachable by Tab (none has tabindex=-1)", async () => {
    const user = userEvent.setup();
    mountPage();
    const dialog = await screen.findByRole("dialog");
    const controls = Array.from(
      dialog.querySelectorAll<HTMLElement>("button, input, a[href]"),
    );
    expect(controls.length).toBeGreaterThanOrEqual(6);
    const reached = new Set<Element>();
    for (let i = 0; i < controls.length + 3; i++) {
      await user.tab();
      if (document.activeElement) reached.add(document.activeElement);
    }
    for (const c of controls) expect(reached.has(c)).toBe(true);
  });

  it("Escape does not record a choice nor write a cookie", async () => {
    const user = userEvent.setup();
    mountPage();
    const dialog = await screen.findByRole("dialog");
    await user.click(dialog.querySelector("input") as HTMLElement);
    await user.keyboard("{Escape}");
    expect(rawCookie()).toBe("");
    expect(readConsent().version).toBe(0);
  });

  it("a choice can be made with the keyboard alone (Enter on Reject all)", async () => {
    const user = userEvent.setup();
    mountPage();
    const reject = await screen.findByRole("button", {
      name: /Từ chối tất cả/i,
    });
    reject.focus();
    await user.keyboard("{Enter}");
    expect(rawCookie()).toBe("v=1&a=0&m=0&p=0&ts=1760000000");
  });
});

describe("Scenario: mounting (layout + footer wiring)", () => {
  const src = (p: string) => readFileSync(join(__dirname, p), "utf8");

  it("root layout mounts ConsentBanner", () => {
    expect(src("../../app/layout.tsx")).toMatch(/<ConsentBanner\b/);
  });

  it("consumer shell footer renders the CookieSettingsLink", () => {
    const shell = src("../../components/shell/ConsumerShell.tsx");
    const footer = shell.slice(
      shell.indexOf("<footer"),
      shell.indexOf("</footer>"),
    );
    expect(footer).toMatch(/<CookieSettingsLink\b/);
  });
});

describe("Scenario: Banner text is Vietnamese", () => {
  it("title is Tùy chọn cookie, groups are Phân tích / Tiếp thị / Cá nhân hóa, no English label remains", async () => {
    mountPage();
    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveAccessibleName("Tùy chọn cookie");
    expect(
      screen.getByRole("heading", { name: "Tùy chọn cookie" }),
    ).toBeVisible();
    for (const g of ["Phân tích", "Tiếp thị", "Cá nhân hóa"]) {
      expect(screen.getByRole("checkbox", { name: g })).toBeInTheDocument();
    }
    expect(dialog.textContent).not.toMatch(
      /Cookie preferences|Analytics|Marketing|Personali[sz]ation|Reject all|Save choices|Accept all|Helps us|Lets us|Tailors/,
    );
  });

  it("footer link reads Cài đặt cookie", () => {
    mountPage();
    expect(
      screen.getByRole("button", { name: "Cài đặt cookie" }),
    ).toBeInTheDocument();
    expect(screen.queryByText("Cookie settings")).not.toBeInTheDocument();
  });
});

describe("Scenario: banner commits are forwarded to the ledger action", () => {
  it("[Signed-in save records only changed purposes] passes the cookie read before the write and the new groups", async () => {
    document.cookie = "agora_consent=v=1&a=1&m=0&p=0&ts=1760000000; Path=/";
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByText(/Cài đặt cookie/i));
    await user.click(screen.getByRole("checkbox", { name: /Cá nhân hóa/i }));
    await user.click(screen.getByRole("button", { name: /Lưu lựa chọn/i }));
    expect(rawCookie()).toBe("v=1&a=1&m=0&p=1&ts=1760000000");
    expect(ledger.recordBannerConsent).toHaveBeenCalledTimes(1);
    const [prev, next] = ledger.recordBannerConsent.mock.calls[0];
    expect(prev).toMatchObject({
      analytics: true,
      marketing: false,
      personalization: false,
      version: 1,
    });
    expect(next).toEqual({
      analytics: true,
      marketing: false,
      personalization: true,
    });
  });

  it("[First choice records all three purposes] with no prior cookie prev has version 0 and next is all denied", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("button", { name: /Từ chối tất cả/i }),
    );
    expect(ledger.recordBannerConsent).toHaveBeenCalledTimes(1);
    const [prev, next] = ledger.recordBannerConsent.mock.calls[0];
    expect(prev).toMatchObject({ version: 0 });
    expect(next).toEqual({
      analytics: false,
      marketing: false,
      personalization: false,
    });
  });

  it("[Anonymous choice is cookie only] Chấp nhận tất cả writes the cookie synchronously with the new format", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("button", { name: /Chấp nhận tất cả/i }),
    );
    expect(rawCookie()).toBe("v=1&a=1&m=1&p=1&ts=1760000000");
  });

  it("[Ledger write failure keeps the cookie choice] a rejected ledger action leaves the cookie, closes the banner, shows no error", async () => {
    ledger.recordBannerConsent.mockRejectedValue(new Error("UNAVAILABLE"));
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("button", { name: /Chấp nhận tất cả/i }),
    );
    expect(rawCookie()).toBe("v=1&a=1&m=1&p=1&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
