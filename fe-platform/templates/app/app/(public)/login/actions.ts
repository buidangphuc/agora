"use server";

import { createGatewayClients, toApiError } from "@fe-platform/api-client";
import { redirect } from "next/navigation";
import { authErrorCode } from "../../../lib/auth-errors";
import { clearSession, safeNext, setSession } from "../../../lib/session";

function back(page: string, error: string, next: string): never {
  redirect(`${page}?error=${error}&next=${encodeURIComponent(next)}`);
}

export async function login(formData: FormData): Promise<void> {
  const next = safeNext(formData.get("next")?.toString());
  const username = formData.get("username")?.toString().trim() ?? "";
  const password = formData.get("password")?.toString() ?? "";
  let failure: ReturnType<typeof toApiError> | undefined;
  let result:
    | Awaited<
        ReturnType<ReturnType<typeof createGatewayClients>["auth"]["login"]>
      >["result"]
    | undefined;
  try {
    result = (await createGatewayClients().auth.login({ username, password }))
      .result;
  } catch (err) {
    failure = toApiError(err);
  }
  if (failure) back("/login", authErrorCode(failure.kind), next);
  if (!result?.token) back("/login", "unavailable", next);
  await setSession({
    token: result.token,
    refreshToken: result.refreshToken,
    refreshExpiresAt: result.refreshExpiresAt,
  });
  redirect(next);
}

export async function logout(): Promise<void> {
  await clearSession();
  redirect("/");
}
