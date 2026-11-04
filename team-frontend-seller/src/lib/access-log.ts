import type { NextRequest } from "next/server";

const HANDOFF_PATH = "/auth/handoff";

export function redactedRequestTarget(url: URL): string {
  if (url.pathname !== HANDOFF_PATH) return url.pathname + url.search;
  const query = url.search
    .replace(/^\?/, "")
    .split("&")
    .filter((p) => p !== "")
    .map((p) => (p.split("=")[0] === "code" ? "code=REDACTED" : p))
    .join("&");
  return query ? `${url.pathname}?${query}` : url.pathname;
}

export function logAccess(req: NextRequest): void {
  console.log(`${req.method} ${redactedRequestTarget(req.nextUrl)}`);
}
