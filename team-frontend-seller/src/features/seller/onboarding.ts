"use server";

import {
  Code,
  ConnectError,
  type Interceptor,
  createPromiseClient,
} from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-node";
import { cookies, headers } from "next/headers";
import { redirect } from "next/navigation";

import { loginPath } from "@/features/auth/returnUrlShared";
import { AuthService } from "@/generated/platform/identity/v1/identity_connect.js";
import { authInterceptor } from "@/lib/gateway/auth";
import { gatewayConfig } from "@/lib/gateway/config";
import { getToken } from "@/lib/gateway/session";
import {
  accessCookieOptions,
  isSecureRequest,
  sessionCookieNames,
} from "@/lib/gateway/session-cookie";

const ONBOARDING_PATH = "/seller/new";
const GENERIC_ERROR = "failed";

export type AcceptSellerTermsResult = { ok?: true; error: string };

function bearer(token: string): Interceptor {
  return (next) => async (req) => {
    req.header.set("authorization", `Bearer ${token}`);
    return next(req);
  };
}

export async function acceptSellerTerms(
  termsVersion: string,
): Promise<AcceptSellerTermsResult> {
  const token = getToken();
  if (!token) return { error: GENERIC_ERROR };

  let newToken = "";
  try {
    const transport = createConnectTransport({
      baseUrl: gatewayConfig.gatewayUrl,
      httpVersion: "1.1",
      interceptors: [authInterceptor({ token }), bearer(token)],
    });
    const res = await createPromiseClient(AuthService, transport).becomeSeller({
      termsVersion,
    });
    newToken = res.result?.token ?? "";
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.PermissionDenied) return { error: "denied" };
      if (err.code === Code.Unauthenticated) {
        redirect(loginPath(ONBOARDING_PATH));
        return { error: GENERIC_ERROR };
      }
    }
    return { error: GENERIC_ERROR };
  }
  if (!newToken) return { error: GENERIC_ERROR };

  const jar = await cookies();
  const h = await headers();
  jar.set(
    sessionCookieNames(h).access,
    newToken,
    accessCookieOptions(isSecureRequest(h)),
  );
  redirect(ONBOARDING_PATH);
  return { ok: true, error: "" };
}
