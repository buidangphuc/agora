"use client";

import type { MouseEvent, ReactNode } from "react";

import { sellerEntryUrl } from "@/lib/apps";

import { startSellerHandoff } from "./actions";

const SELLER_HOME = "/seller";

export function SellerChannelLink({
  children,
  className,
}: {
  children?: ReactNode;
  className?: string;
}) {
  const fallback = sellerEntryUrl(SELLER_HOME);

  async function onClick(e: MouseEvent<HTMLAnchorElement>) {
    e.preventDefault();
    let target = fallback;
    try {
      target = (await startSellerHandoff(SELLER_HOME)) || fallback;
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
