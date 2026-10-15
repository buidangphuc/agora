"use client";

import { useEffect } from "react";

import { trackEcommerce } from "@/lib/analytics";

const COOKIE = "agora_auth_evt";

/** Delete the cookie first, then fire the matching event if the value is known. */
function consume() {
  const m = new RegExp(`(?:^|;\\s*)${COOKIE}=([^;]*)`).exec(document.cookie);
  if (!m) return;
  document.cookie = `${COOKIE}=; Max-Age=0; path=/`;
  const value = m[1];
  if (value === "sign_up" || value === "login") {
    trackEcommerce(value, { properties: { method: "password" } });
  }
}

/**
 * One-shot consumer of the auth event cookie set by loginAction/registerAction.
 * The shop layout stays mounted across the post-login soft redirect, so besides
 * mount it also consumes on history navigation; a missing cookie makes any
 * later run a no-op.
 */
export function AuthEventBeacon() {
  useEffect(() => {
    consume();
    const { pushState, replaceState } = window.history;
    const wrap =
      (orig: History["pushState"]): History["pushState"] =>
      (...args) => {
        orig.apply(window.history, args);
        consume();
      };
    const push = wrap(pushState);
    const replace = wrap(replaceState);
    window.history.pushState = push;
    window.history.replaceState = replace;
    return () => {
      if (window.history.pushState === push)
        window.history.pushState = pushState;
      if (window.history.replaceState === replace)
        window.history.replaceState = replaceState;
    };
  }, []);
  return null;
}
