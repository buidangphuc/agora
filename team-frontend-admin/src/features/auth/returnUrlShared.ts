const MAX_LEN = 2048;

/**
 * Returns `raw` when it is a safe same-origin relative path, otherwise "/".
 * Guards against open redirects and login/register loops.
 */
export function safeReturnUrl(raw: string | null | undefined): string {
  if (typeof raw !== "string" || raw === "" || raw.length > MAX_LEN) return "/";
  if (raw[0] !== "/" || raw[1] === "/" || raw[1] === "\\") return "/";
  // biome-ignore lint/suspicious/noControlCharactersInRegex: rejecting control chars is the point
  if (/[\u0000-\u001f\u007f\\]/.test(raw)) return "/";
  if (/%0d|%0a|%09|%5c/i.test(raw)) return "/";
  if (/^\/%2f%2f/i.test(raw)) return "/";
  const path = raw.split(/[?#]/, 1)[0].toLowerCase();
  if (/^\/(login|register)(\/|$)/.test(path)) return "/";
  return raw;
}

export function loginPath(returnTo?: string | null): string {
  if (returnTo && safeReturnUrl(returnTo) === returnTo) {
    return `/login?returnUrl=${encodeURIComponent(returnTo)}`;
  }
  return "/login";
}
