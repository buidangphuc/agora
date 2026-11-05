"use client";

import type { MouseEvent, ReactNode } from "react";

import { buyerAppUrl } from "@/lib/apps";

import { startBuyerHandoff } from "./actions";

const BUYER_HOME = "/";

function loginUrl(next: string): string {
  return buyerAppUrl(`/login?next=${encodeURIComponent(next)}`);
}

export function BuyerStorefrontLink({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  const fallback = loginUrl(BUYER_HOME);

  async function onClick(e: MouseEvent<HTMLAnchorElement>) {
    e.preventDefault();
    let target = fallback;
    try {
      target = (await startBuyerHandoff(BUYER_HOME)) || fallback;
    } catch {
      target = fallback;
    }
    window.location.assign(target);
  }

  return (
    <a href={fallback} className={className} onClick={onClick}>
      {children}
    </a>
  );
}
