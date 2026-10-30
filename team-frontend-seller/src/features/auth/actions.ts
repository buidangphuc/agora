"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";

import { makeClients } from "@/lib/gateway/client";
import { clientContextFrom } from "@/lib/gateway/client-context";
import { refreshSession } from "@/lib/gateway/refresh";
import { revokeOwnSession, sessionIdOf } from "@/lib/gateway/revoke";
import {
  accessCookieOptions,
  isSecureRequest,
  refreshCookieOptions,
  sessionCookieNames,
} from "@/lib/gateway/session-cookie";
import { isUsableToken } from "@/lib/gateway/token-expiry";

import { safeReturnUrl } from "./returnUrl";
import {
  type CredentialErrors,
  validateLogin,
  validateRegister,
} from "./validation";

/**
 * Result of loginAction / registerAction. On success they redirect (the one
 * documented exception to the `{ ok, error?, data? }` action contract:
 * redirect() throws and never returns); on failure they resolve with
 * `ok: false`, a form-level `error` and, when the server names a field,
 * `fields` for that FormItem's help text.
 */
export interface AuthState {
  ok: boolean;
  error?: string;
  fields?: CredentialErrors;
  // The username that was submitted, so the form can restore it after a
  // failure without keeping the input controlled (see useCredentialForm).
  username?: string;
}

function failure(
  username: string,
  error: string,
  fields?: CredentialErrors,
): AuthState {
  return fields
    ? { ok: false, error, fields, username }
    : { ok: false, error, username };
}

const AUTH_EVT_COOKIE = "agora_auth_evt";
const AUTH_EVT_OPTS = (secure: boolean) =>
  ({
    maxAge: 60,
    sameSite: "lax",
    path: "/",
    httpOnly: false,
    secure,
  }) as const;

async function setSession(token: string, refreshToken: string) {
  const jar = await cookies();
  const h = await headers();
  const secure = isSecureRequest(h);
  const names = sessionCookieNames(h);
  jar.set(names.access, token, accessCookieOptions(secure));
  if (refreshToken) {
    jar.set(names.refresh, refreshToken, refreshCookieOptions(secure));
  }
}

export async function loginAction(
  _prev: AuthState,
  formData: FormData,
): Promise<AuthState> {
  const username = String(formData.get("username") ?? "").trim();
  const password = String(formData.get("password") ?? "");

  const missing = validateLogin({ username, password });
  if (Object.keys(missing).length > 0) {
    return failure(
      username,
      "Vui lòng nhập đầy đủ thông tin đăng nhập.",
      missing,
    );
  }

  let token = "";
  let refreshToken = "";
  try {
    const res = await makeClients(undefined, {
      client: clientContextFrom(await headers()),
    }).auth.login({ username, password });
    token = res.result?.token ?? "";
    refreshToken = res.result?.refreshToken ?? "";
  } catch (err) {
    if (err instanceof ConnectError) {
      if (
        err.code === Code.Unauthenticated ||
        err.code === Code.InvalidArgument
      ) {
        return failure(
          username,
          "Tên đăng nhập hoặc mật khẩu không chính xác.",
        );
      }
      if (err.code === Code.PermissionDenied) {
        return failure(
          username,
          "Tài khoản của bạn đã bị khóa. Vui lòng liên hệ bộ phận hỗ trợ.",
        );
      }
      return failure(
        username,
        `Đăng nhập không thành công: ${err.rawMessage || "Lỗi dịch vụ xác thực"}`,
      );
    }
    return failure(username, "Không thể kết nối đến máy chủ xác thực.");
  }
  if (!token) return failure(username, "Không nhận được phiên đăng nhập.");
  await setSession(token, refreshToken);
  (await cookies()).set(
    AUTH_EVT_COOKIE,
    "login",
    AUTH_EVT_OPTS(isSecureRequest(await headers())),
  );
  redirect(safeReturnUrl(String(formData.get("returnUrl") ?? "")));
}

export async function registerAction(
  _prev: AuthState,
  formData: FormData,
): Promise<AuthState> {
  const username = String(formData.get("username") ?? "").trim();
  const password = String(formData.get("password") ?? "");
  const role = String(formData.get("role") ?? "buyer");

  const invalid = validateRegister({ username, password });
  if (Object.keys(invalid).length > 0) {
    return failure(
      username,
      "Tên đăng nhập ≥ 3 ký tự, mật khẩu ≥ 4 ký tự.",
      invalid,
    );
  }

  let token = "";
  let refreshToken = "";
  try {
    const res = await makeClients(undefined, {
      client: clientContextFrom(await headers()),
    }).auth.register({ username, password, role });
    token = res.result?.token ?? "";
    refreshToken = res.result?.refreshToken ?? "";
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.AlreadyExists) {
      return failure(username, "Tên đăng nhập đã tồn tại.", {
        username: "Tên đăng nhập đã tồn tại.",
      });
    }
    return failure(username, `Đăng ký lỗi: ${String(err)}`);
  }
  if (!token) return failure(username, "Không nhận được phiên đăng nhập.");
  await setSession(token, refreshToken);
  (await cookies()).set(
    AUTH_EVT_COOKIE,
    "sign_up",
    AUTH_EVT_OPTS(isSecureRequest(await headers())),
  );
  redirect(safeReturnUrl(String(formData.get("returnUrl") ?? "")));
}

export async function logoutAction() {
  const jar = await cookies();
  const names = sessionCookieNames(await headers());
  try {
    const client = clientContextFrom(await headers());
    let bearer: string | undefined;
    const access = jar.get(names.access)?.value;
    const refresh = jar.get(names.refresh)?.value;
    if (access && isUsableToken(access)) {
      bearer = access;
    } else if (refresh) {
      bearer = (await refreshSession(refresh, client)).token || undefined;
    }
    const sid = bearer ? sessionIdOf(bearer) : undefined;
    if (bearer && sid) await revokeOwnSession(sid, bearer, client);
  } catch {
    // Best effort: logout must always clear the cookies.
  }
  jar.delete(names.access);
  jar.delete(names.refresh);
  redirect("/");
}
