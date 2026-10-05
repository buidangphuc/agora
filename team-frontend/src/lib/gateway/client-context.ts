/**
 * The browser's network identity as this Next.js server sees it, to be forwarded to
 * the gateway so a new session records the real device and IP (ADR-0003 addendum).
 *
 * Why this exists: the gateway's peer for browser traffic is THIS server, not the
 * browser. The gateway believes an `X-Forwarded-For` only from peers listed in its
 * TRUSTED_PROXIES (the frontend's service), so what we put there must be derived
 * with care:
 *  - the IP is NEVER taken from the leftmost X-Forwarded-For entry (the client
 *    controls that); it is the entry `TRUSTED_PROXY_HOPS` places from the right,
 *    i.e. the address the nearest trusted reverse proxy saw. With no proxy in
 *    front, Next fills X-Forwarded-For with the socket address itself (only when the
 *    client did not send one);
 *  - the user agent is the browser's own header — audit data a client can set
 *    freely anyway, so it carries no trust.
 * Both are audit-only downstream: never used for authorization.
 */
import { isIP } from "node:net";

export interface ClientContext {
  ip?: string;
  userAgent?: string;
}

interface HeaderReader {
  get(name: string): string | null | undefined;
}

const MAX_UA_LENGTH = 256;

/** Reverse proxies in front of this server that append to X-Forwarded-For (>= 1). */
function proxyHops(env: Record<string, string | undefined>): number {
  const n = Number.parseInt(env.TRUSTED_PROXY_HOPS ?? "", 10);
  return Number.isFinite(n) && n >= 1 ? n : 1;
}

export function clientContextFrom(
  h: HeaderReader,
  env: Record<string, string | undefined> = process.env,
): ClientContext {
  const out: ClientContext = {};

  const forwarded = (h.get("x-forwarded-for") ?? "")
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
  if (forwarded.length > 0) {
    const idx = Math.max(0, forwarded.length - proxyHops(env));
    const candidate = forwarded[idx] ?? "";
    if (isIP(candidate) !== 0) out.ip = candidate;
  }

  const ua = (h.get("user-agent") ?? "").trim();
  if (ua) out.userAgent = ua.slice(0, MAX_UA_LENGTH);

  return out;
}
