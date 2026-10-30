import {
  type ApiError,
  type ApiErrorKind,
  toApiError,
} from "@fe-platform/api-client";
import { cookies, headers } from "next/headers";
import { notFound, redirect } from "next/navigation";
import type { SessionConfig } from "./config";
import { cookieNames, cookieOptions, isSecureRequest } from "./cookies";
import {
  type Principal,
  decodePrincipal,
  hasScopes,
  isUsableToken,
} from "./token";

export function safeNext(next: string | null | undefined): string {
  if (!next || !next.startsWith("/")) return "/";
  if (next.startsWith("//") || next.includes("\\")) return "/";
  return next;
}

export type Attempt<T> =
  | { ok: true; value: T }
  | { ok: false; code: ApiErrorKind };

export function createSessionServer<C>(
  cfg: SessionConfig,
  createClients: (token: string) => C,
) {
  const loginUrl = (next: string) =>
    `${cfg.loginPath}?next=${encodeURIComponent(safeNext(next))}`;

  async function names() {
    return cookieNames(cfg, isSecureRequest(await headers()));
  }

  async function getToken(): Promise<string | undefined> {
    const raw = (await cookies()).get((await names()).access)?.value;
    return raw && isUsableToken(raw) ? raw : undefined;
  }

  async function getPrincipal(): Promise<Principal | null> {
    const token = await getToken();
    return token ? decodePrincipal(token) : null;
  }

  async function requireSession(next = "/"): Promise<string> {
    const token = await getToken();
    if (!token) redirect(loginUrl(next));
    return token;
  }

  async function requireAccess(next = "/"): Promise<string> {
    const token = await requireSession(next);
    if (!hasScopes(decodePrincipal(token), cfg.requiredScopes)) {
      redirect(cfg.forbiddenPath);
    }
    return token;
  }

  async function setSession(result: {
    token: string;
    refreshToken?: string;
    refreshExpiresAt?: { toDate(): Date };
  }): Promise<void> {
    const secure = isSecureRequest(await headers());
    const n = cookieNames(cfg, secure);
    const jar = await cookies();
    jar.set(n.access, result.token, cookieOptions(cfg, "access", secure));
    if (result.refreshToken) {
      jar.set(n.refresh, result.refreshToken, {
        ...cookieOptions(cfg, "refresh", secure),
        ...(result.refreshExpiresAt
          ? { expires: result.refreshExpiresAt.toDate() }
          : {}),
      });
    }
  }

  async function clearSession(): Promise<void> {
    const n = await names();
    const jar = await cookies();
    jar.delete(n.access);
    jar.delete(n.refresh);
  }

  async function guarded<T>(
    next: string,
    fn: (clients: C) => Promise<T>,
  ): Promise<T> {
    const token = await requireAccess(next);
    let failure: ApiError;
    try {
      return await fn(createClients(token));
    } catch (err) {
      failure = toApiError(err);
    }
    if (failure.kind === "unauthenticated") redirect(loginUrl(next));
    if (failure.kind === "forbidden") redirect(cfg.forbiddenPath);
    if (failure.kind === "not_found") notFound();
    throw failure;
  }

  async function attempt<T>(
    next: string,
    fn: (clients: C) => Promise<T>,
  ): Promise<Attempt<T>> {
    const token = await requireAccess(next);
    let failure: ApiError;
    try {
      return { ok: true, value: await fn(createClients(token)) };
    } catch (err) {
      failure = toApiError(err);
    }
    if (failure.kind === "unauthenticated") redirect(loginUrl(next));
    return { ok: false, code: failure.kind };
  }

  return {
    getToken,
    getPrincipal,
    requireSession,
    requireAccess,
    setSession,
    clearSession,
    guarded,
    attempt,
  };
}
