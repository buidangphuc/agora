import type { NextRequest } from "next/server";

const HANDOFF_PATHS = new Set(["/auth/handoff", "/auth/handoff/"]);
const HANDOFF_KEYS = new Set(["code"]);
const SELLER_KEYS = new Set(["q", "cursor"]);

function keyName(param: string): string | null {
  const raw = param.split("=")[0].replace(/\+/g, " ");
  try {
    return decodeURIComponent(raw);
  } catch {
    return null;
  }
}

function sensitiveKeys(pathname: string): Set<string> | null {
  if (HANDOFF_PATHS.has(pathname)) return HANDOFF_KEYS;
  if (pathname === "/seller" || pathname.startsWith("/seller/")) {
    return SELLER_KEYS;
  }
  return null;
}

export function redactedRequestTarget(url: URL): string {
  const keys = sensitiveKeys(url.pathname);
  if (!keys) return url.pathname + url.search;
  const params = url.search
    .replace(/^\?/, "")
    .split("&")
    .filter((p) => p !== "");
  const names = params.map(keyName);
  const whole = `${[...keys][0]}=REDACTED`;
  const query = names.includes(null)
    ? whole
    : params
        .map((p, i) => {
          const name = names[i] as string;
          return keys.has(name) ? `${name}=REDACTED` : p;
        })
        .join("&");
  return query ? `${url.pathname}?${query}` : url.pathname;
}

export function logAccess(req: NextRequest): void {
  console.log(`${req.method} ${redactedRequestTarget(req.nextUrl)}`);
}
