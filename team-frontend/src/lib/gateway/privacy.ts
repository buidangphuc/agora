import "server-only";

import { ConsentPurpose } from "@/generated/platform/identity/v1/identity_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export type ConsentPurposeName =
  | "ANALYTICS"
  | "MARKETING"
  | "PERSONALIZATION"
  | "TERMS"
  | "PRIVACY_POLICY";

export interface ConsentEntry {
  purpose: ConsentPurposeName;
  granted: boolean;
  version: string;
  source: string;
}

export interface ConsentPage {
  records: ConsentEntry[];
  nextCursor: string;
}

const PAGE_SIZE = 100;

export async function listConsents(cursor = ""): Promise<ConsentPage> {
  const res = await makeClients(getToken()).privacy.listMyConsents({
    page: { cursor, pageSize: PAGE_SIZE },
  });
  return {
    records: res.records.map((r) => ({
      purpose: (ConsentPurpose[r.purpose] ??
        "UNSPECIFIED") as ConsentPurposeName,
      granted: r.granted,
      version: r.version,
      source: r.source,
    })),
    nextCursor: res.page?.nextCursor ?? "",
  };
}

export async function recordConsents(records: ConsentEntry[]): Promise<void> {
  await makeClients(getToken()).privacy.recordConsent({
    records: records.map((r) => ({
      purpose: ConsentPurpose[r.purpose],
      granted: r.granted,
      version: r.version,
      source: r.source,
    })),
  });
}
