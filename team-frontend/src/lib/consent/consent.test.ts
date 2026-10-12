// Scenarios: openspec/changes/add-consent-mode/specs/consent/spec.md
// Requirement "Consent cookie format".
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  COOKIE_NAME,
  MAX_AGE_SECONDS,
  parseConsent,
  readConsent,
  serializeConsent,
  writeConsent,
} from "./consent";

const DENIED = {
  analytics: false,
  marketing: false,
  personalization: false,
  version: 0,
};
const T = 1760000000;

function clearCookie() {
  document.cookie = `${COOKIE_NAME}=; Path=/; Max-Age=0`;
}

// jsdom's document.cookie drops attributes on read, so capture raw writes.
function captureCookieWrites(): { writes: string[]; restore: () => void } {
  const writes: string[] = [];
  Object.defineProperty(document, "cookie", {
    configurable: true,
    get: () => "",
    set: (v: string) => {
      writes.push(v);
    },
  });
  return {
    writes,
    restore: () => {
      // biome-ignore lint/performance/noDelete: restore prototype accessor
      delete (document as unknown as { cookie?: string }).cookie;
    },
  };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date(T * 1000));
  clearCookie();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  clearCookie();
});

describe("constants (scenario: Boundary at the 180-day lifetime)", () => {
  it("cookie name and Max-Age are fixed", () => {
    expect(COOKIE_NAME).toBe("agora_consent");
    expect(MAX_AGE_SECONDS).toBe(15552000);
    expect(MAX_AGE_SECONDS).toBe(180 * 24 * 60 * 60);
  });
});

describe("serializeConsent", () => {
  it("Accept all writes the cookie: value v=1&a=1&m=1&p=1&ts=T", () => {
    expect(
      serializeConsent({ analytics: true, marketing: true, personalization: true }, T),
    ).toBe(`v=1&a=1&m=1&p=1&ts=${T}`);
  });
  it("Reject all writes the cookie: value v=1&a=0&m=0&p=0&ts=T", () => {
    expect(
      serializeConsent({ analytics: false, marketing: false, personalization: false }, T),
    ).toBe(`v=1&a=0&m=0&p=0&ts=${T}`);
  });
  it("Granular choice is stored per group: only personalization", () => {
    expect(
      serializeConsent({ analytics: false, marketing: false, personalization: true }, T),
    ).toBe(`v=1&a=0&m=0&p=1&ts=${T}`);
  });
  it("Flags are independent: each group maps to its own key", () => {
    expect(serializeConsent({ analytics: true, marketing: false, personalization: false }, T)).toBe(`v=1&a=1&m=0&p=0&ts=${T}`);
    expect(serializeConsent({ analytics: false, marketing: true, personalization: false }, T)).toBe(`v=1&a=0&m=1&p=0&ts=${T}`);
  });
  it("Oversized cookie: serialized value stays within 256 bytes and has no ';'", () => {
    const v = serializeConsent({ analytics: true, marketing: true, personalization: true }, T);
    expect(v.length).toBeLessThanOrEqual(256);
    expect(v).not.toContain(";");
  });
});

describe("parseConsent", () => {
  it("Flags are independent: p=1 only -> personalization true, version 1", () => {
    expect(parseConsent(`v=1&a=0&m=0&p=1&ts=${T}`)).toEqual({
      analytics: false,
      marketing: false,
      personalization: true,
      version: 1,
    });
  });
  it("Flags are independent: a=1 only", () => {
    expect(parseConsent(`v=1&a=1&m=0&p=0&ts=${T}`)).toEqual({
      analytics: true,
      marketing: false,
      personalization: false,
      version: 1,
    });
  });
  it("Flags are independent: m=1 only", () => {
    expect(parseConsent(`v=1&a=0&m=1&p=0&ts=${T}`)).toMatchObject({
      analytics: false,
      marketing: true,
      personalization: false,
      version: 1,
    });
  });
  it("Accept all round trip", () => {
    expect(parseConsent(`v=1&a=1&m=1&p=1&ts=${T}`)).toEqual({
      analytics: true,
      marketing: true,
      personalization: true,
      version: 1,
    });
  });
  it("absent cookie is all denied, version 0", () => {
    expect(parseConsent(undefined)).toEqual(DENIED);
    expect(parseConsent(null)).toEqual(DENIED);
    expect(parseConsent("")).toEqual(DENIED);
  });
  it("Unknown version is treated as denied", () => {
    expect(parseConsent(`v=2&a=1&m=1&p=1&ts=${T}`)).toEqual(DENIED);
    expect(parseConsent(`v=0&a=1&m=1&p=1&ts=${T}`)).toEqual(DENIED);
  });
  it("Missing fields are treated as denied", () => {
    expect(parseConsent("a=1&m=1")).toEqual(DENIED);
    expect(parseConsent(`v=1&a=1&m=1&ts=${T}`)).toEqual(DENIED);
    expect(parseConsent(`v=1&m=1&p=1&ts=${T}`)).toEqual(DENIED);
    expect(parseConsent(`a=1&m=1&p=1&ts=${T}`)).toEqual(DENIED);
  });
  it("Non-0/1 flag is treated as denied", () => {
    expect(parseConsent(`v=1&a=2&m=0&p=0&ts=${T}`)).toEqual(DENIED);
    expect(parseConsent(`v=1&a=1&m=yes&p=1&ts=${T}`)).toEqual(DENIED);
    expect(parseConsent(`v=1&a=1&m=1&p=&ts=${T}`)).toEqual(DENIED);
  });
  it("Non-numeric or missing ts is treated as denied", () => {
    expect(parseConsent("v=1&a=1&m=1&p=1&ts=abc")).toEqual(DENIED);
    expect(parseConsent("v=1&a=1&m=1&p=1")).toEqual(DENIED);
    expect(parseConsent("v=1&a=1&m=1&p=1&ts=")).toEqual(DENIED);
  });
  it("Oversized cookie is treated as denied", () => {
    const big = `v=1&a=1&m=1&p=1&ts=${"1".repeat(240)}`;
    expect(big.length).toBeGreaterThan(256);
    expect(parseConsent(big)).toEqual(DENIED);
  });
  it("malformed garbage is denied", () => {
    expect(parseConsent("%%%&&&===")).toEqual(DENIED);
  });
});

describe("writeConsent", () => {
  it("Accept all writes the cookie with ts = current unix seconds", () => {
    const res = writeConsent({ analytics: true, marketing: true, personalization: true });
    expect(res).toMatchObject({ analytics: true, marketing: true, personalization: true, version: 1 });
    expect(document.cookie).toContain(`${COOKIE_NAME}=v=1&a=1&m=1&p=1&ts=${T}`);
  });
  it("Reject all writes the cookie", () => {
    writeConsent({ analytics: false, marketing: false, personalization: false });
    expect(document.cookie).toContain(`${COOKIE_NAME}=v=1&a=0&m=0&p=0&ts=${T}`);
  });
  it("Granular choice is stored per group", () => {
    writeConsent({ analytics: false, marketing: false, personalization: true });
    expect(document.cookie).toContain(`${COOKIE_NAME}=v=1&a=0&m=0&p=1&ts=${T}`);
  });
  it("Boundary at the 180-day lifetime: Path, Max-Age, SameSite=Lax, no HttpOnly, no Secure on http", () => {
    const cap = captureCookieWrites();
    try {
      writeConsent({ analytics: true, marketing: false, personalization: false }, { secure: false });
    } finally {
      cap.restore();
    }
    expect(cap.writes).toHaveLength(1);
    const [value, ...attrs] = cap.writes[0].split(";").map((s) => s.trim());
    expect(value).toBe(`${COOKIE_NAME}=v=1&a=1&m=0&p=0&ts=${T}`);
    expect(attrs).toContain("Path=/");
    expect(attrs).toContain("Max-Age=15552000");
    expect(attrs).toContain("SameSite=Lax");
    expect(attrs.map((a) => a.toLowerCase())).not.toContain("httponly");
    expect(attrs).not.toContain("Secure");
  });
  it("Boundary at the 180-day lifetime: Secure on https", () => {
    const cap = captureCookieWrites();
    try {
      writeConsent({ analytics: true, marketing: true, personalization: true }, { secure: true });
    } finally {
      cap.restore();
    }
    const attrs = cap.writes[0].split(";").map((s) => s.trim());
    expect(attrs).toContain("Secure");
    expect(attrs).toContain("Max-Age=15552000");
  });
  it("Revoking analytics saves a=0 rather than removing the cookie", () => {
    writeConsent({ analytics: true, marketing: true, personalization: true });
    writeConsent({ analytics: false, marketing: true, personalization: true });
    expect(readConsent()).toMatchObject({ analytics: false, marketing: true, version: 1 });
    expect(document.cookie).toContain(`${COOKIE_NAME}=v=1&a=0&m=1&p=1&ts=${T}`);
  });
  it("SSR-safe: no document -> no throw", () => {
    vi.stubGlobal("document", undefined);
    expect(() =>
      writeConsent({ analytics: true, marketing: true, personalization: true }),
    ).not.toThrow();
  });
});

describe("readConsent", () => {
  it("returns denied/version 0 when there is no cookie (banner shown)", () => {
    expect(readConsent()).toEqual(DENIED);
  });
  it("Flags are independent: reads document.cookie among other cookies", () => {
    document.cookie = "other=1; Path=/";
    document.cookie = `${COOKIE_NAME}=v=1&a=0&m=0&p=1&ts=${T}; Path=/`;
    document.cookie = "zzz=2; Path=/";
    expect(readConsent()).toEqual({
      analytics: false,
      marketing: false,
      personalization: true,
      version: 1,
    });
  });
  it("Unknown version in the browser cookie is denied", () => {
    document.cookie = `${COOKIE_NAME}=v=2&a=1&m=1&p=1&ts=${T}; Path=/`;
    expect(readConsent()).toEqual(DENIED);
  });
  it("accepts a Cookie header string (server side) and ignores other cookies", () => {
    expect(readConsent(`foo=bar; ${COOKIE_NAME}=v=1&a=1&m=0&p=0&ts=${T}; baz=1`)).toEqual({
      analytics: true,
      marketing: false,
      personalization: false,
      version: 1,
    });
    expect(readConsent("foo=bar")).toEqual(DENIED);
  });
  it("SSR-safe: no document and no header -> denied, no throw", () => {
    vi.stubGlobal("document", undefined);
    expect(readConsent()).toEqual(DENIED);
  });
});
