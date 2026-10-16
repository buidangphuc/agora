export const SESSION_COOKIE = "session";
export const REFRESH_COOKIE = "agora_refresh";

const ACCESS_MAX_AGE_S = 3600;
const REFRESH_MAX_AGE_S = 2592000;

function cookieOptions(maxAge: number, secure: boolean) {
  return {
    httpOnly: true,
    sameSite: "lax" as const,
    path: "/",
    maxAge,
    secure,
  };
}

export function accessCookieOptions(secure: boolean) {
  return cookieOptions(ACCESS_MAX_AGE_S, secure);
}

export function refreshCookieOptions(secure: boolean) {
  return cookieOptions(REFRESH_MAX_AGE_S, secure);
}

export function isSecureRequest(headers: Headers): boolean {
  return headers.get("x-forwarded-proto") === "https";
}
