import "server-only";
import { redirectToLogin } from "./returnUrl";

const MAX_LEN = 2048;

export type GuardSearchParams = Record<string, string | string[] | undefined>;

/**
 * Rebuilds the guarded page's path + query for the login returnUrl.
 * Returns undefined (plain /login) when the result exceeds the length cap.
 */
export function buildReturnPath(
  pathname: string,
  searchParams?: GuardSearchParams | null,
): string | undefined {
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
