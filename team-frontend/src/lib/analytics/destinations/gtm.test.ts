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
    Reflect.deleteProperty(window, "dataLayer");
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
    Reflect.deleteProperty(window, "dataLayer");
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
    window.history.replaceState(null, "", "/search?q=ao&returnUrl=%2Fx&page=2");
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

describe("trackPageLocation (client-side navigation; NEW export of ./gtm)", () => {
  const latestPageLocation = () => {
    const vals = ((window.dataLayer ?? []) as unknown[])
      .map((e) => (e as { page_location?: unknown } | null)?.page_location)
      .filter((v): v is string => typeof v === "string");
    return vals[vals.length - 1];
  };

  beforeEach(() => {
    document.head.innerHTML = "";
    Reflect.deleteProperty(window, "dataLayer");
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    window.history.replaceState(null, "", "/");
  });

  it("[page_location has no returnUrl] client navigation to /login?returnUrl pushes a page_location without returnUrl", async () => {
    const { trackPageLocation } = await import("./gtm");
    applyConsent(ALL);
    window.history.pushState(null, "", "/login?returnUrl=%2Fx");
    trackPageLocation();
    expect(latestPageLocation()).toBe(`${window.location.origin}/login`);
    expect(JSON.stringify(window.dataLayer)).not.toContain("returnUrl");
  });

  it("[Other query parameters are not removed] client navigation keeps other params", async () => {
    const { trackPageLocation } = await import("./gtm");
    applyConsent(ALL);
    window.history.pushState(null, "", "/search?q=ao&returnUrl=%2Fx&page=2");
    trackPageLocation();
    expect(latestPageLocation()).toBe(
      `${window.location.origin}/search?q=ao&page=2`,
    );
  });
});

describe("applyConsent page_location (admin paths are path only)", () => {
  const pageLocations = () =>
    ((window.dataLayer ?? []) as unknown[])
      .map((e) => (e as { page_location?: unknown } | null)?.page_location)
      .filter((v): v is string => typeof v === "string");

  const locationFor = (url: string): string => {
    document.head.innerHTML = "";
    Reflect.deleteProperty(window, "dataLayer");
    window.history.replaceState(null, "", url);
    applyConsent(ALL);
    const all = pageLocations();
    expect(all).toHaveLength(1);
    return all[0];
  };

  beforeEach(() => {
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    window.history.replaceState(null, "", "/");
  });

  it("[Referrer hygiene: admin page_location is path only] admin page_location drops all query and fragment (also for a non-admin principal on an admin URL)", () => {
    const loc = locationFor("/admin/audit?target_id=u-1&actor_id=u-2#x");
    expect(loc).toBe(`${window.location.origin}/admin/audit`);
    const dump = JSON.stringify(window.dataLayer);
    expect(dump).not.toContain("target_id");
    expect(dump).not.toContain("u-2");

    expect(locationFor("/admin/users?query=x")).toBe(
      `${window.location.origin}/admin/users`,
    );
  });

  it("[Referrer hygiene: look-alike path is not treated as admin] admin match is segment exact", () => {
    expect(locationFor("/administrator?q=1")).toBe(
      `${window.location.origin}/administrator?q=1`,
    );
    expect(locationFor("/adminx?q=1")).toBe(
      `${window.location.origin}/adminx?q=1`,
    );
  });

  it("[Referrer hygiene: the admin root is stripped too] admin root and case variants stripped", () => {
    for (const url of ["/admin?x=1", "/ADMIN/users?x=1", "/admin/users/?x=1"]) {
      const loc = locationFor(url);
      expect(loc).not.toContain("?");
      expect(loc).not.toContain("x=1");
      expect(loc.startsWith(window.location.origin)).toBe(true);
    }
    expect(locationFor("/admin?x=1")).toBe(`${window.location.origin}/admin`);
  });

  it("[Referrer hygiene: non-admin pages keep other params] keeps other params, only returnUrl removed", () => {
    expect(locationFor("/listing/abc?variant=v2&returnUrl=%2Fx")).toBe(
      `${window.location.origin}/listing/abc?variant=v2`,
    );
  });

  it("[Referrer hygiene: login page_location still has no returnUrl] strips returnUrl on login", () => {
    const loc = locationFor("/login?returnUrl=%2Faccount%2Forders%2Fo-9");
    expect(loc).toBe(`${window.location.origin}/login`);
    expect(loc).not.toContain("o-9");
  });
});

describe("page_referrer (scrubbed, pushed with every page_location)", () => {
  type Push = { page_location?: string; page_referrer?: string };
  const pagePushes = (): Push[] =>
    ((window.dataLayer ?? []) as unknown[]).filter(
      (e): e is Push =>
        !!e &&
        typeof e === "object" &&
        typeof (e as Push).page_location === "string",
    );
  const setReferrer = (ref: string) =>
    Object.defineProperty(document, "referrer", {
      value: ref,
      configurable: true,
    });

  beforeEach(() => {
    document.head.innerHTML = "";
    Reflect.deleteProperty(window, "dataLayer");
    vi.stubEnv("NEXT_PUBLIC_GTM_ID", "GTM-TEST");
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    Reflect.deleteProperty(document, "referrer");
    window.history.replaceState(null, "", "/");
  });

  it("[Referrer hygiene: login URL referrer loses returnUrl] page_referrer drops login returnUrl", () => {
    setReferrer(
      "https://shop.example/login?returnUrl=%2Fadmin%2Fusers%3Fquery%3Da%40b.vn",
    );
    applyConsent(ALL);
    const pushes = pagePushes();
    expect(pushes).toHaveLength(1);
    expect(pushes[0].page_referrer).toBe("https://shop.example/login");
    const dump = JSON.stringify(window.dataLayer);
    expect(dump).not.toContain("returnUrl");
    expect(dump).not.toContain("b.vn");
  });

  it("[Referrer hygiene: admin referrer loses its query] page_referrer drops admin query", () => {
    setReferrer("https://shop.example/admin/audit?target_id=u-1");
    applyConsent(ALL);
    expect(pagePushes()[0].page_referrer).toBe(
      "https://shop.example/admin/audit",
    );
    expect(JSON.stringify(window.dataLayer)).not.toContain("target_id");
  });

  it.each(["", "not a url"])(
    "[Referrer hygiene: empty or invalid referrer becomes empty string] referrer %j gives page_referrer empty string, no throw",
    (ref) => {
      setReferrer(ref);
      expect(() => applyConsent(ALL)).not.toThrow();
      const pushes = pagePushes();
      expect(pushes).toHaveLength(1);
      expect(pushes[0].page_referrer).toBe("");
    },
  );

  it("[Referrer hygiene: login URL referrer loses returnUrl] client navigation push also carries the scrubbed page_referrer", async () => {
    const { trackPageLocation } = await import("./gtm");
    setReferrer("https://shop.example/admin/audit?target_id=u-1");
    applyConsent(ALL);
    window.history.pushState(null, "", "/search?q=ao");
    trackPageLocation();
    const pushes = pagePushes();
    const last = pushes[pushes.length - 1];
    expect(last.page_location).toBe(`${window.location.origin}/search?q=ao`);
    expect(last.page_referrer).toBe("https://shop.example/admin/audit");
  });

  it("[Referrer hygiene: no push without analytics consent] no page_location and no page_referrer without consent", async () => {
    const { trackPageLocation } = await import("./gtm");
    setReferrer("https://shop.example/admin/audit?target_id=u-1");
    applyConsent(NONE);
    trackPageLocation();
    expect(pagePushes()).toHaveLength(0);
    expect(JSON.stringify(window.dataLayer)).not.toContain("page_referrer");
    expect(JSON.stringify(window.dataLayer)).not.toContain("target_id");
  });
});
