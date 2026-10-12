import { readFileSync } from "node:fs";
import { join } from "node:path";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readConsent } from "@/lib/consent/consent";

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
    for (const g of [/analytics/i, /marketing/i, /personali[sz]ation/i]) {
      expect(
        await screen.findByRole("checkbox", { name: g }),
      ).toBeInTheDocument();
    }
    expect(dialog).toHaveAccessibleName();
    for (const b of [/accept all/i, /reject all/i, /save choices/i]) {
      expect(screen.getByRole("button", { name: b })).toBeInTheDocument();
    }
  });

  it("starts with every optional group unchecked (opt-in)", async () => {
    mountPage();
    for (const g of [/analytics/i, /marketing/i, /personali[sz]ation/i]) {
      expect(await screen.findByRole("checkbox", { name: g })).not.toBeChecked();
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
    await user.click(await screen.findByRole("button", { name: /accept all/i }));
    expect(rawCookie()).toBe("v=1&a=1&m=1&p=1&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("Reject all writes v=1&a=0&m=0&p=0 and removes the banner", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByRole("button", { name: /reject all/i }));
    expect(rawCookie()).toBe("v=1&a=0&m=0&p=0&ts=1760000000");
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
  });

  it("Save choices with only personalization writes a=0&m=0&p=1", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(
      await screen.findByRole("checkbox", { name: /personali[sz]ation/i }),
    );
    await user.click(screen.getByRole("button", { name: /save choices/i }));
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
      (el) => el.tagName !== "FOOTER" && !el.closest(".fixed") && !el.classList.contains("fixed"),
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
    await screen.findByText(/cookie settings/i);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("footer 'Cookie settings' link re-opens the banner", async () => {
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByText(/cookie settings/i));
    expect(await screen.findByRole("dialog")).toBeInTheDocument();
  });

  it("re-opened banner reflects stored choices and saving updates the cookie", async () => {
    document.cookie = "agora_consent=v=1&a=0&m=0&p=1&ts=1760000000; Path=/";
    const user = userEvent.setup();
    mountPage();
    await user.click(await screen.findByText(/cookie settings/i));
    expect(
      await screen.findByRole("checkbox", { name: /personali[sz]ation/i }),
    ).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: /analytics/i }));
    await user.click(screen.getByRole("button", { name: /save choices/i }));
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
    const reject = await screen.findByRole("button", { name: /reject all/i });
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
    const footer = shell.slice(shell.indexOf("<footer"), shell.indexOf("</footer>"));
    expect(footer).toMatch(/<CookieSettingsLink\b/);
  });
});
