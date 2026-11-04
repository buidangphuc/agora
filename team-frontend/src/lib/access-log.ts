import type { NextRequest } from "next/server";

const HANDOFF_PATHS = new Set(["/auth/handoff", "/auth/handoff/"]);
const REDACTED = "code=REDACTED";

function keyName(param: string): string | null {
  const raw = param.split("=")[0].replace(/\+/g, " ");
  try {
    return decodeURIComponent(raw);
  } catch {
    return null;
  }
}

export function redactedRequestTarget(url: URL): string {
  if (!HANDOFF_PATHS.has(url.pathname)) return url.pathname + url.search;
  const params = url.search
    .replace(/^\?/, "")
    .split("&")
    .filter((p) => p !== "");
  const names = params.map(keyName);
  const query = names.includes(null)
    ? REDACTED
    : params.map((p, i) => (names[i] === "code" ? REDACTED : p)).join("&");
  return query ? `${url.pathname}?${query}` : url.pathname;
}

export function logAccess(req: NextRequest): void {
  console.log(`${req.method} ${redactedRequestTarget(req.nextUrl)}`);
}
