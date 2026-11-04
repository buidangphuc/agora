"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";

import { redirectToLogin } from "@/features/auth/returnUrl";
import { makeClients } from "@/lib/gateway/client";
import { getToken } from "@/lib/gateway/session";
import {
  accessCookieOptions,
  isSecureRequest,
  refreshCookieOptions,
  sessionCookieNames,
} from "@/lib/gateway/session-cookie";

const SELLER_HOME = "/seller/new";
const FAILED = "Không thể hoàn tất, thử lại sau";
const TERMS_REJECTED = "Không thể xác nhận điều khoản";

export async function acceptSellerTerms(
  termsVersion: string,
): Promise<{ error: string }> {
  const token = getToken();
  if (!token) return redirectToLogin(SELLER_HOME);

  let access = "";
  let refresh = "";
  try {
    const res = await makeClients(token).auth.becomeSeller({ termsVersion });
    access = res.result?.token ?? "";
    refresh = res.result?.refreshToken ?? "";
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.PermissionDenied) return { error: "denied" };
      if (err.code === Code.Unauthenticated) {
        return redirectToLogin(SELLER_HOME);
      }
      if (err.code === Code.InvalidArgument) return { error: TERMS_REJECTED };
    }
    return { error: FAILED };
  }
  if (!access) return { error: FAILED };

  const jar = await cookies();
  const h = await headers();
  const secure = isSecureRequest(h);
  const names = sessionCookieNames(h);
  jar.set(names.access, access, accessCookieOptions(secure));
  if (refresh) {
    jar.set(names.refresh, refresh, refreshCookieOptions(secure));
  }
  redirect(SELLER_HOME);
}
