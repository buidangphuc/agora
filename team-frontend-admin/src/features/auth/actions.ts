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
import { type CredentialErrors, validateLogin } from "./validation";

export interface AuthState {
  ok: boolean;
  error?: string;
  fields?: CredentialErrors;
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

async function setSession(token: string, refreshToken: string) {
  const h = await headers();
  const secure = isSecureRequest(h);
  const names = sessionCookieNames(h);
  const jar = await cookies();
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
      return failure(username, "Đăng nhập không thành công. Vui lòng thử lại.");
    }
    return failure(username, "Không thể kết nối đến máy chủ xác thực.");
  }
  if (!token) return failure(username, "Không nhận được phiên đăng nhập.");
  await setSession(token, refreshToken);
  redirect(safeReturnUrl(String(formData.get("returnUrl") ?? "")));
}

export async function logoutAction() {
  const h = await headers();
  const names = sessionCookieNames(h);
  const jar = await cookies();
  try {
    const client = clientContextFrom(h);
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
  } catch {}
  jar.delete(names.access);
  jar.delete(names.refresh);
  redirect("/login");
}
