"use server";

import { startHandoff } from "@fe-platform/session";

import { sellerEntryUrl } from "@/lib/apps";

import { buyerHandoffSession, buyerOrigin, sellerOrigin } from "./config";
import { mintHandoffCode } from "./mint";

export async function startSellerHandoff(next: string): Promise<string> {
  try {
    return await startHandoff(buyerHandoffSession, "seller", next, {
      mint: mintHandoffCode,
      origins: { buyer: buyerOrigin(), seller: sellerOrigin() },
    });
  } catch {
    return sellerEntryUrl(next);
  }
}
