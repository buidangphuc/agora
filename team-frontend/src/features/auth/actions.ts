"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { makeClients } from "@/lib/gateway/client";
import { SESSION_COOKIE } from "@/lib/gateway/session";

export interface AuthState {
  error?: string;
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
        return { error: "Tên đăng nhập hoặc mật khẩu không chính xác." };
      }
      return {
        error: `Đăng nhập không thành công: ${err.rawMessage || "Lỗi dịch vụ xác thực"}`,
      };
    }
    return { error: "Không thể kết nối đến máy chủ xác thực." };
  }
  if (!token) return { error: "Không nhận được phiên đăng nhập." };
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

  if (username.length < 3 || password.length < 4) {
    return { error: "Tên đăng nhập ≥ 3 ký tự, mật khẩu ≥ 4 ký tự." };
  }

  let token = "";
  try {
    const res = await makeClients().auth.register({ username, password, role });
    token = res.result?.token ?? "";
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.AlreadyExists) {
      return { error: "Tên đăng nhập đã tồn tại." };
    }
    return { error: `Đăng ký lỗi: ${String(err)}` };
  }
  if (!token) return { error: "Không nhận được phiên đăng nhập." };
  setSession(token);
  redirect("/");
}

export async function logoutAction() {
  cookies().delete(SESSION_COOKIE);
  redirect("/");
}
