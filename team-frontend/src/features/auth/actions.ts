"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { makeClients } from "@/lib/gateway/client";
import { SESSION_COOKIE } from "@/lib/gateway/session";

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
}

function failure(error: string, fields?: CredentialErrors): AuthState {
  return fields ? { ok: false, error, fields } : { ok: false, error };
}

function setSession(token: string) {
  cookies().set(SESSION_COOKIE, token, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    maxAge: 3600,
  });
}

export async function loginAction(
  _prev: AuthState,
  formData: FormData,
): Promise<AuthState> {
  const username = String(formData.get("username") ?? "").trim();
  const password = String(formData.get("password") ?? "");

  const missing = validateLogin({ username, password });
  if (Object.keys(missing).length > 0) {
    return failure("Vui lòng nhập đầy đủ thông tin đăng nhập.", missing);
  }

  let token = "";
  try {
    const res = await makeClients().auth.login({ username, password });
    token = res.result?.token ?? "";
  } catch (err) {
    if (err instanceof ConnectError) {
      if (
        err.code === Code.Unauthenticated ||
        err.code === Code.InvalidArgument
      ) {
        return failure("Tên đăng nhập hoặc mật khẩu không chính xác.");
      }
      return failure(
        `Đăng nhập không thành công: ${err.rawMessage || "Lỗi dịch vụ xác thực"}`,
      );
    }
    return failure("Không thể kết nối đến máy chủ xác thực.");
  }
  if (!token) return failure("Không nhận được phiên đăng nhập.");
  setSession(token);
  redirect("/");
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
    return failure("Tên đăng nhập ≥ 3 ký tự, mật khẩu ≥ 4 ký tự.", invalid);
  }

  let token = "";
  try {
    const res = await makeClients().auth.register({ username, password, role });
    token = res.result?.token ?? "";
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.AlreadyExists) {
      return failure("Tên đăng nhập đã tồn tại.", {
        username: "Tên đăng nhập đã tồn tại.",
      });
    }
    return failure(`Đăng ký lỗi: ${String(err)}`);
  }
  if (!token) return failure("Không nhận được phiên đăng nhập.");
  setSession(token);
  redirect("/");
}

export async function logoutAction() {
  cookies().delete(SESSION_COOKIE);
  redirect("/");
}
