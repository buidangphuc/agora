import "server-only";
import { headers } from "next/headers";
import { redirectToLogin } from "./returnUrl";

const MAX_LEN = 2048;

export type GuardSearchParams = Record<string, string | string[] | undefined>;

/** Segment-exact match: /admin and /admin/..., not /administrator. */
export function isAdminPath(pathname: string): boolean {
  return pathname === "/admin" || pathname.startsWith("/admin/");
}

/**
 * Rebuilds the guarded page's path + query for the login returnUrl.
 * Returns undefined (plain /login) when the result exceeds the length cap.
 */
export function buildReturnPath(
  pathname: string,
  searchParams?: GuardSearchParams | null,
): string | undefined {
  if (isAdminPath(pathname)) {
    return pathname.length > MAX_LEN ? undefined : pathname;
  }
  const parts: string[] = [];
  for (const [key, value] of Object.entries(searchParams ?? {})) {
    if (value === undefined) continue;
    for (const v of Array.isArray(value) ? value : [value]) {
      parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(v)}`);
    }
  }
  const full = parts.length > 0 ? `${pathname}?${parts.join("&")}` : pathname;
  return full.length > MAX_LEN ? undefined : full;
}

/** Guest guard for pages with a known pathname (and optional searchParams). */
export function redirectGuestToLogin(
  pathname: string,
  searchParams?: GuardSearchParams | null,
): never {
  return redirectToLogin(buildReturnPath(pathname, searchParams));
}

/** Guest guard for param-less layouts: reads the `x-pathname` request header. */
export async function redirectGuestToLoginFromHeader(): Promise<never> {
  const h = await headers();
  const path = h.get("x-pathname");
  if (!path || path.length > MAX_LEN) return redirectToLogin(undefined);
  const pathname = path.split(/[?#]/, 1)[0];
  return redirectToLogin(isAdminPath(pathname) ? pathname : path);
}
