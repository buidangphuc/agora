"use server";

import { listConsents, recordConsents } from "@/lib/gateway/privacy";
import { getToken } from "@/lib/gateway/session";

const MAX_PAGES = 5;

export type LegalAcceptance =
  | { signedIn: false }
  | { signedIn: true; needsAcceptance: boolean };

function versions() {
  return {
    terms: process.env.NEXT_PUBLIC_LEGAL_TERMS_VERSION ?? "2026-11-13",
    privacy: process.env.NEXT_PUBLIC_PRIVACY_POLICY_VERSION ?? "2026-11-13",
  };
}

export async function getLegalAcceptance(): Promise<LegalAcceptance> {
  if (!getToken()) return { signedIn: false };
  const want = versions();
  try {
    let terms: string | undefined;
    let privacy: string | undefined;
    let cursor = "";
    for (let i = 0; i < MAX_PAGES; i++) {
      const page = await listConsents(cursor);
      for (const r of page.records) {
        if (r.purpose === "TERMS" && terms === undefined) terms = r.version;
        if (r.purpose === "PRIVACY_POLICY" && privacy === undefined) {
          privacy = r.version;
        }
      }
      if ((terms !== undefined && privacy !== undefined) || !page.nextCursor) {
        break;
      }
      cursor = page.nextCursor;
    }
    return {
      signedIn: true,
      needsAcceptance: terms !== want.terms || privacy !== want.privacy,
    };
  } catch {
    return { signedIn: true, needsAcceptance: false };
  }
}

export async function acceptCurrentLegal(): Promise<{ ok: boolean }> {
  if (!getToken()) return { ok: false };
  const want = versions();
  try {
    await recordConsents([
      {
        purpose: "TERMS",
        granted: true,
        version: want.terms,
        source: "account",
      },
      {
        purpose: "PRIVACY_POLICY",
        granted: true,
        version: want.privacy,
        source: "account",
      },
    ]);
    return { ok: true };
  } catch {
    return { ok: false };
  }
}
