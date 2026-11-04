"use server";

import { startHandoff } from "@fe-platform/session";

import {
  buyerLoginUrl,
  buyerOrigin,
  sellerOrigin,
  sellerSession,
} from "./config";
import { mintHandoffCode } from "./mint";

export async function startBuyerHandoff(next: string): Promise<string> {
  try {
    return await startHandoff(sellerSession, "buyer", next, {
      mint: mintHandoffCode,
      origins: { buyer: buyerOrigin(), seller: sellerOrigin() },
    });
  } catch {
    return buyerLoginUrl(next);
  }
}
