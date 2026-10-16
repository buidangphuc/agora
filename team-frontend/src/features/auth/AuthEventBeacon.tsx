"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { trackEcommerce } from "@/lib/analytics";

const COOKIE = "agora_auth_evt";

/** Delete the cookie first, then fire the matching event if the value is known. */
function consume() {
  const m = new RegExp(`(?:^|;\\s*)${COOKIE}=([^;]*)`).exec(document.cookie);
  if (!m) return;
  document.cookie = `${COOKIE}=; Max-Age=0; path=/; SameSite=Lax`;
  const value = m[1];
  if (value === "sign_up" || value === "login") {
    trackEcommerce(value, { properties: { method: "password" } });
  }
}

/**
 * One-shot consumer of the auth event cookie set by loginAction/registerAction.
 * The shop layout stays mounted across the post-login soft redirect, so the
 * pathname re-runs the consumer; a missing cookie makes later runs no-ops.
 */
export function AuthEventBeacon() {
  const pathname = usePathname();
  // biome-ignore lint/correctness/useExhaustiveDependencies: pathname is a re-run trigger for the soft redirect after login; consume() reads only document.cookie
  useEffect(consume, [pathname]);
  return null;
}
