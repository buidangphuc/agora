import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { applyConsent } from "./gtm";

const gtmScripts = () =>
  Array.from(document.querySelectorAll("script")).filter((s) =>
    s.src.includes("googletagmanager.com"),
  );

type Entry = [string, string, Record<string, string>];
const consentEntries = (): Entry[] =>
  ((window.dataLayer ?? []) as unknown[])
    .map((e) =>
      e && typeof e === "object" && "length" in e
        ? Array.from(e as ArrayLike<unknown>)
        : e,
    )
    .filter((e): e is Entry => Array.isArray(e) && e[0] === "consent");

const isArguments = (e: unknown) =>
  Object.prototype.toString.call(e) === "[object Arguments]";
const rawConsentEntries = () =>
  ((window.dataLayer ?? []) as unknown[]).filter((e) => {
    const a =
      e && typeof e === "object" && "length" in e
        ? Array.from(e as ArrayLike<unknown>)
        : e;
    return Array.isArray(a) && a[0] === "consent";
  });

const ALL = { analytics: true, marketing: true, personalization: true };
const NONE = { analytics: false, marketing: false, personalization: false };

describe("applyConsent (Consent Mode v2 basic gating)", () => {
  beforeEach(() => {
    document.head.innerHTML = "";
    document.body.innerHTML = "";
    delete (window as { dataLayer?: unknown }).dataLayer;
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  it("[Accept analytics loads GTM and updates consent] default(all denied) then update(analytics granted) precede exactly 1 GTM script", () => {
    let seenAtInject: Entry[] | null = null;
    const orig = document.head.appendChild.bind(document.head);
    vi.spyOn(document.head, "appendChild").mockImplementation(((n: Node) => {
      if ((n as HTMLScriptElement).src?.includes("googletagmanager.com")) {
        seenAtInject = consentEntries();
      }
      return orig(n);
    }) as typeof document.head.appendChild);

    applyConsent(ALL);

    const entries = consentEntries();
    expect(entries.map((e) => e[1])).toEqual(["default", "update"]);
    expect(Object.keys(entries[0][2]).length).toBeGreaterThan(0);
    expect(Object.values(entries[0][2]).every((v) => v === "denied")).toBe(
      true,
    );
    expect(entries[0][2]).toHaveProperty("analytics_storage");
    expect(entries[0][2]).toHaveProperty("ad_storage");
    expect(entries[1][2].analytics_storage).toBe("granted");
    expect(gtmScripts()).toHaveLength(1);
    expect(gtmScripts()[0].src).toContain("GTM-TEST");
    // both consent entries were already in the dataLayer when the script was injected
    expect(seenAtInject).not.toBeNull();
    expect((seenAtInject as unknown as Entry[]).map((e) => e[1])).toEqual([
      "default",
      "update",
    ]);
  });

  it("[Marketing and personalization map to signals] analytics+marketing, no personalization", () => {
    applyConsent({ analytics: true, marketing: true, personalization: false });
    const update = consentEntries().find((e) => e[1] === "update");
    expect(update?.[2]).toMatchObject({
      analytics_storage: "granted",
      ad_storage: "granted",
      ad_user_data: "granted",
      ad_personalization: "granted",
      personalization_storage: "denied",
    });
  });

  it("[Marketing and personalization map to signals] analytics+personalization, no marketing", () => {
    applyConsent({ analytics: true, marketing: false, personalization: true });
    const update = consentEntries().find((e) => e[1] === "update");
    expect(update?.[2]).toMatchObject({
      analytics_storage: "granted",
      ad_storage: "denied",
      ad_user_data: "denied",
      ad_personalization: "denied",
      personalization_storage: "granted",
    });
  });

  it("[Returning consented buyer loads GTM once] repeated applyConsent (client navigation) keeps 1 script and 1 default", () => {
    applyConsent(ALL);
    applyConsent(ALL);
    applyConsent(ALL);
    expect(gtmScripts()).toHaveLength(1);
    expect(consentEntries().filter((e) => e[1] === "default")).toHaveLength(1);
  });

  it("[Rejected analytics never loads GTM] analytics=false adds no script, even with marketing granted", () => {
    applyConsent(NONE);
    applyConsent({ analytics: false, marketing: true, personalization: true });
    expect(gtmScripts()).toHaveLength(0);
    expect(
      consentEntries().some(
        (e) => e[1] === "update" && e[2].analytics_storage === "granted",
      ),
    ).toBe(false);
  });

  it.each(["", "   "])(
    "[Empty GTM id keeps GTM off even with consent] NEXT_PUBLIC_GTM_ID=%j adds no script",
    (id) => {
      vi.stubEnv("NEXT_PUBLIC_GTM_ID", id);
      applyConsent(ALL);
      expect(gtmScripts()).toHaveLength(0);
    },
  );

  it("[Empty GTM id keeps GTM off even with consent] unset id adds no script", () => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", undefined as unknown as string);
    applyConsent(ALL);
    expect(gtmScripts()).toHaveLength(0);
  });

  it("[Revoking analytics stops pushes] after a grant, analytics=false pushes update analytics_storage=denied last", () => {
    applyConsent(ALL);
    applyConsent(NONE);
    const entries = consentEntries();
    const last = entries[entries.length - 1];
    expect(last[1]).toBe("update");
    expect(last[2].analytics_storage).toBe("denied");
    expect(last[2].ad_storage).toBe("denied");
    expect(gtmScripts()).toHaveLength(1);
  });

  it("[Accept analytics loads GTM and updates consent] consent commands are Arguments objects (gtag/GTM ignore plain arrays)", () => {
    applyConsent(ALL);
    const raw = rawConsentEntries();
    expect(raw).toHaveLength(2);
    expect(raw.every(isArguments)).toBe(true);
  });

  it("[Revoking analytics stops pushes] the denied update is an Arguments object", () => {
    applyConsent(ALL);
    applyConsent(NONE);
    const raw = rawConsentEntries();
    expect(raw.length).toBeGreaterThanOrEqual(3);
    expect(raw.every(isArguments)).toBe(true);
  });
});

describe("applyConsent debug_mode (staging only)", () => {
  const debugIdx = () =>
    ((window.dataLayer ?? []) as unknown[]).findIndex(
      (e) =>
        !!e &&
        typeof e === "object" &&
        (e as { debug_mode?: unknown }).debug_mode === true,
    );
  const updateIdx = () =>
    ((window.dataLayer ?? []) as unknown[]).findIndex((e) => {
      const a =
        e && typeof e === "object" && "length" in e
          ? Array.from(e as ArrayLike<unknown>)
          : [];
      return a[0] === "consent" && a[1] === "update";
    });

  beforeEach(() => {
    document.head.innerHTML = "";
    (window as { dataLayer?: unknown }).dataLayer = undefined;
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => vi.unstubAllEnvs());

  it("[Staging events carry debug_mode] staging + analytics consent pushes an entry with debug_mode: true after the consent update", () => {
    vi.stubEnv("NEXT_PUBLIC_APP_ENV", "staging");
    applyConsent(ALL);
    expect(debugIdx()).toBeGreaterThan(-1);
    expect(debugIdx()).toBeGreaterThan(updateIdx());
  });

  it("[Staging events carry debug_mode] repeated applyConsent keeps exactly 1 debug_mode entry", () => {
    vi.stubEnv("NEXT_PUBLIC_APP_ENV", "staging");
    applyConsent(ALL);
    applyConsent(ALL);
    const n = ((window.dataLayer ?? []) as unknown[]).filter(
      (e) => (e as { debug_mode?: unknown })?.debug_mode === true,
    ).length;
    expect(n).toBe(1);
  });

  it("[Staging events carry debug_mode] staging without analytics consent pushes no debug_mode", () => {
    vi.stubEnv("NEXT_PUBLIC_APP_ENV", "staging");
    applyConsent(NONE);
    expect(debugIdx()).toBe(-1);
  });

  it.each(["production", "", "development"])(
    "[Production events do not carry debug_mode] NEXT_PUBLIC_APP_ENV=%j pushes no debug_mode",
    (env) => {
      vi.stubEnv("NEXT_PUBLIC_APP_ENV", env);
      applyConsent(ALL);
      expect(debugIdx()).toBe(-1);
    },
  );
});

describe("applyConsent page_location (returnUrl stripping)", () => {
  const pageLocations = () =>
    ((window.dataLayer ?? []) as unknown[])
      .map((e) => (e as { page_location?: unknown } | null)?.page_location)
      .filter((v): v is string => typeof v === "string");

  beforeEach(() => {
    document.head.innerHTML = "";
    delete (window as { dataLayer?: unknown }).dataLayer;
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    window.history.replaceState(null, "", "/");
  });

  it("[page_location has no returnUrl] strips returnUrl from page_location", () => {
    window.history.replaceState(
      null,
      "",
      "/login?returnUrl=%2Faccount%2Forders%2Fo-9",
    );
    applyConsent(ALL);
    expect(pageLocations()).toEqual([`${window.location.origin}/login`]);
    const dump = JSON.stringify(window.dataLayer);
    expect(dump).not.toContain("returnUrl");
    expect(dump).not.toContain("o-9");
  });

  it("[Other query parameters are not removed] keeps other params", () => {
    window.history.replaceState(
      null,
      "",
      "/search?q=ao&returnUrl=%2Fx&page=2",
    );
    applyConsent(ALL);
    expect(pageLocations()).toEqual([
      `${window.location.origin}/search?q=ao&page=2`,
    ]);
  });

  it("[Other query parameters are not removed] a URL without returnUrl is not changed", () => {
    window.history.replaceState(null, "", "/search?q=ao&page=2");
    applyConsent(ALL);
    expect(pageLocations()).toEqual([
      `${window.location.origin}/search?q=ao&page=2`,
    ]);
  });
});
