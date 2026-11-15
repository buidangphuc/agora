"use client";

import type { ReactNode } from "react";

const ENDPOINT = "/api/ads/click";

function send(token: string) {
  const body = JSON.stringify({ token });
  try {
    if (
      typeof navigator !== "undefined" &&
      typeof navigator.sendBeacon === "function"
    ) {
      const blob = new Blob([body], { type: "application/json" });
      if (navigator.sendBeacon(ENDPOINT, blob)) return;
    }
    void fetch(ENDPOINT, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body,
      keepalive: true,
    }).catch(() => undefined);
  } catch {
    return;
  }
}

function tokenOf(target: EventTarget | null): string | null {
  if (!(target instanceof Element)) return null;
  const slot = target.closest("[data-impression-token]");
  return slot?.getAttribute("data-impression-token") || null;
}

export function SponsoredClickGuard({ children }: { children: ReactNode }) {
  return (
    <div
      style={{ display: "contents" }}
      onClick={(e) => {
        const token = tokenOf(e.target);
        if (token) send(token);
      }}
      onAuxClick={(e) => {
        if (e.button !== 1) return;
        const token = tokenOf(e.target);
        if (token) send(token);
      }}
    >
      {children}
    </div>
  );
}
