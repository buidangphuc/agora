import "server-only";

import { HandoffApp } from "@/generated/platform/identity/v1/identity_pb.js";
import { makeClients } from "@/lib/gateway/client";

type Target = "buyer" | "seller";

function handoffApp(target: Target): HandoffApp {
  return target === "seller" ? HandoffApp.SELLER : HandoffApp.BUYER;
}

export async function mintHandoffCode(
  token: string,
  target: Target,
): Promise<{ code: string }> {
  const res = await makeClients(token).auth.createHandoffCode({
    targetApp: handoffApp(target),
  });
  return { code: res.code };
}

export async function peekHandoffCode(
  code: string,
  target: Target,
): Promise<{ username: string }> {
  const res = await makeClients().auth.peekHandoffCode({
    code,
    targetApp: handoffApp(target),
  });
  return { username: res.username };
}

export async function redeemHandoffCode(code: string, target: Target) {
  const res = await makeClients().auth.redeemHandoffCode({
    code,
    targetApp: handoffApp(target),
  });
  return {
    token: res.result?.token ?? "",
    refreshToken: res.result?.refreshToken,
    refreshExpiresAt: res.result?.refreshExpiresAt,
  };
}
