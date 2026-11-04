import { cookies, headers } from "next/headers";
import type { SessionConfig } from "./config";
import { cookieNames, cookieOptions, isSecureRequest } from "./cookies";
import { safeNext } from "./server";
import { isUsableToken } from "./token";

const MAX_CODE_LENGTH = 64;
const MAX_NAME_LENGTH = 64;
const HANDOFF_PATH = "/auth/handoff";

type HandoffTarget = "buyer" | "seller";

export interface HandoffHandlerDeps {
  origin: string;
  peek: (code: string) => Promise<{ username: string }>;
  redeem: (code: string) => Promise<{
    token: string;
    refreshToken?: string;
    refreshExpiresAt?: { toDate(): Date };
  }>;
}

export interface StartHandoffDeps {
  mint: (token: string, target: HandoffTarget) => Promise<{ code: string }>;
  origins: Record<HandoffTarget, string>;
}

function assertEnabled(cfg: SessionConfig): void {
  if (cfg.app === "admin") throw new Error("hand-off is not available");
}

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function truncate(value: string): string {
  const chars = Array.from(value);
  return chars.length > MAX_NAME_LENGTH
    ? `${chars.slice(0, MAX_NAME_LENGTH - 1).join("")}…`
    : value;
}

function baseHeaders(): Headers {
  const h = new Headers();
  h.set("referrer-policy", "no-referrer");
  h.set("cache-control", "no-store");
  return h;
}

function redirectTo(location: string): Response {
  const h = baseHeaders();
  h.set("location", location);
  return new Response(null, { status: 303, headers: h });
}

function confirmationPage(username: string, code: string, next: string) {
  const h = baseHeaders();
  h.set("content-type", "text/html; charset=utf-8");
  h.set(
    "content-security-policy",
    "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'",
  );
  h.set("x-frame-options", "DENY");
  const html = `<!doctype html><html lang="vi"><head><meta charset="utf-8"><meta name="referrer" content="no-referrer"><meta name="robots" content="noindex"><title>Xác nhận đăng nhập</title></head><body><main><h1>Tiếp tục với tài khoản ${escapeHtml(truncate(username))}?</h1><form action="${HANDOFF_PATH}" method="post"><input type="hidden" name="code" value="${escapeHtml(code)}"><input type="hidden" name="next" value="${escapeHtml(next)}"><button type="submit">Tiếp tục</button></form></main></body></html>`;
  return new Response(html, { status: 200, headers: h });
}

function validCode(code: unknown): code is string {
  return (
    typeof code === "string" &&
    code.length > 0 &&
    code.length <= MAX_CODE_LENGTH
  );
}

function sameOriginRequest(req: Request, origin: string): boolean {
  const claimed = req.headers.get("origin");
  const site = req.headers.get("sec-fetch-site");
  if (claimed === null && site === null) return false;
  if (claimed !== null && claimed !== origin) return false;
  if (site !== null && site !== "same-origin") return false;
  return true;
}

export function createHandoffHandler(
  cfg: SessionConfig,
  deps: HandoffHandlerDeps,
) {
  assertEnabled(cfg);

  const loginUrl = (next?: string) =>
    next === undefined
      ? cfg.loginPath
      : `${cfg.loginPath}?next=${encodeURIComponent(safeNext(next))}`;

  async function GET(req: Request): Promise<Response> {
    const url = new URL(req.url);
    const code = url.searchParams.get("code");
    const next = safeNext(url.searchParams.get("next"));
    if (!validCode(code)) return redirectTo(loginUrl(next));
    try {
      const { username } = await deps.peek(code);
      return confirmationPage(username, code, next);
    } catch {
      return redirectTo(loginUrl(next));
    }
  }

  async function HEAD(req: Request): Promise<Response> {
    const res = await GET(req);
    return new Response(null, { status: res.status, headers: res.headers });
  }

  async function POST(req: Request): Promise<Response> {
    if (!sameOriginRequest(req, deps.origin)) return redirectTo(loginUrl());
    let code: unknown;
    let nextRaw: unknown;
    try {
      const form = await req.formData();
      code = form.get("code");
      nextRaw = form.get("next");
    } catch {
      return redirectTo(loginUrl());
    }
    const next = safeNext(typeof nextRaw === "string" ? nextRaw : null);
    if (!validCode(code)) return redirectTo(loginUrl(next));
    try {
      const result = await deps.redeem(code);
      const secure = isSecureRequest(await headers());
      const names = cookieNames(cfg, secure);
      const jar = await cookies();
      jar.set(names.access, result.token, cookieOptions(cfg, "access", secure));
      if (result.refreshToken) {
        jar.set(names.refresh, result.refreshToken, {
          ...cookieOptions(cfg, "refresh", secure),
          ...(result.refreshExpiresAt
            ? { expires: result.refreshExpiresAt.toDate() }
            : {}),
        });
      }
      return redirectTo(next);
    } catch {
      return redirectTo(loginUrl(next));
    }
  }

  return { GET, HEAD, POST };
}

export async function startHandoff(
  cfg: SessionConfig,
  target: HandoffTarget,
  next: string,
  deps: StartHandoffDeps,
): Promise<string> {
  assertEnabled(cfg);
  const safe = safeNext(next);
  const origin = deps.origins[target];
  const fallback = `${origin}${cfg.loginPath}?next=${encodeURIComponent(safe)}`;
  const names = cookieNames(cfg, isSecureRequest(await headers()));
  const token = (await cookies()).get(names.access)?.value;
  if (!token || !isUsableToken(token)) return fallback;
  try {
    const { code } = await deps.mint(token, target);
    return `${origin}${HANDOFF_PATH}?code=${encodeURIComponent(code)}&next=${encodeURIComponent(safe)}`;
  } catch {
    return fallback;
  }
}
