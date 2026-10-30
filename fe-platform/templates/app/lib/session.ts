import "server-only";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

export const SESSION_COOKIE = "session";
export const REFRESH_COOKIE = "refresh";
export const REFRESHED_COOKIE = "refreshed";

const secure = () => process.env.NODE_ENV === "production";

export async function getToken(): Promise<string | undefined> {
  const jar = await cookies();
  return jar.get(SESSION_COOKIE)?.value || undefined;
}

export async function getRefreshToken(): Promise<string | undefined> {
  const jar = await cookies();
  return jar.get(REFRESH_COOKIE)?.value || undefined;
}

export function safeNext(next: string | null | undefined): string {
  if (!next || !next.startsWith("/")) return "/";
  if (next.startsWith("//") || next.includes("\\")) return "/";
  return next;
}

export async function requireSession(next = "/"): Promise<string> {
  const token = await getToken();
  if (!token) redirect(`/login?next=${encodeURIComponent(safeNext(next))}`);
  return token;
}

export async function setSession(result: {
  token: string;
  refreshToken: string;
  refreshExpiresAt?: { toDate(): Date };
}): Promise<void> {
  const jar = await cookies();
  const base = {
    httpOnly: true,
    sameSite: "lax" as const,
    path: "/",
    secure: secure(),
  };
  jar.set(SESSION_COOKIE, result.token, base);
  if (result.refreshToken) {
    jar.set(REFRESH_COOKIE, result.refreshToken, {
      ...base,
      expires: result.refreshExpiresAt?.toDate(),
    });
  }
}

export async function clearSession(): Promise<void> {
  const jar = await cookies();
  jar.delete(SESSION_COOKIE);
  jar.delete(REFRESH_COOKIE);
}
