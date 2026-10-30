"use server";

import { createGatewayClients, toApiError } from "@fe-platform/api-client";
import { redirect } from "next/navigation";
import { authErrorCode } from "../../../lib/auth-errors";
import { safeNext, setSession } from "../../../lib/session";

export async function register(formData: FormData): Promise<void> {
  const next = safeNext(formData.get("next")?.toString());
  const username = formData.get("username")?.toString().trim() ?? "";
  const password = formData.get("password")?.toString() ?? "";
  let failure: ReturnType<typeof toApiError> | undefined;
  let result:
    | Awaited<
        ReturnType<ReturnType<typeof createGatewayClients>["auth"]["register"]>
      >["result"]
    | undefined;
  try {
    result = (
      await createGatewayClients().auth.register({ username, password })
    ).result;
  } catch (err) {
    failure = toApiError(err);
  }
  const fail = (code: string): never =>
    redirect(`/register?error=${code}&next=${encodeURIComponent(next)}`);
  if (failure)
    fail(failure.kind === "invalid" ? "taken" : authErrorCode(failure.kind));
  if (!result?.token) fail("unavailable");
  await setSession({
    token: result?.token ?? "",
    refreshToken: result?.refreshToken ?? "",
    refreshExpiresAt: result?.refreshExpiresAt,
  });
  redirect(next);
}
