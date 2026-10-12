export const COOKIE_NAME = "agora_consent";
export const MAX_AGE_SECONDS = 15552000; // 180 days
const MAX_VALUE_LENGTH = 256;

export interface ConsentGroups {
  analytics: boolean;
  marketing: boolean;
  personalization: boolean;
}

export type Consent = ConsentGroups & { version: 0 | 1 };

const denied = (): Consent => ({
  analytics: false,
  marketing: false,
  personalization: false,
  version: 0,
});

export function parseConsent(raw: string | null | undefined): Consent {
  if (!raw || raw.length > MAX_VALUE_LENGTH) return denied();
  const fields = new Map<string, string>();
  for (const part of raw.split("&")) {
    const i = part.indexOf("=");
    if (i <= 0) continue;
    fields.set(part.slice(0, i), part.slice(i + 1));
  }
  const flag = (k: string): boolean | undefined => {
    const v = fields.get(k);
    return v === "1" ? true : v === "0" ? false : undefined;
  };
  const a = flag("a");
  const m = flag("m");
  const p = flag("p");
  if (
    fields.get("v") !== "1" ||
    a === undefined ||
    m === undefined ||
    p === undefined ||
    !/^\d+$/.test(fields.get("ts") ?? "")
  ) {
    return denied();
  }
  return { analytics: a, marketing: m, personalization: p, version: 1 };
}

export function serializeConsent(
  groups: ConsentGroups,
  tsSeconds: number,
): string {
  const b = (x: boolean) => (x ? "1" : "0");
  return `v=1&a=${b(groups.analytics)}&m=${b(groups.marketing)}&p=${b(groups.personalization)}&ts=${tsSeconds}`;
}

export function readConsent(cookieHeader?: string): Consent {
  const header =
    cookieHeader ??
    (typeof document === "undefined" || !document ? "" : document.cookie);
  for (const part of header.split(";")) {
    const s = part.trim();
    if (s.startsWith(`${COOKIE_NAME}=`)) {
      return parseConsent(s.slice(COOKIE_NAME.length + 1));
    }
  }
  return denied();
}

export function writeConsent(
  groups: ConsentGroups,
  opts: { secure?: boolean } = {},
): Consent {
  const ts = Math.floor(Date.now() / 1000);
  const value = serializeConsent(groups, ts);
  if (typeof document !== "undefined" && document) {
    const secure =
      opts.secure ??
      (typeof location !== "undefined" && location.protocol === "https:");
    document.cookie = `${COOKIE_NAME}=${value}; Path=/; Max-Age=${MAX_AGE_SECONDS}; SameSite=Lax${secure ? "; Secure" : ""}`;
  }
  return parseConsent(value);
}
