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

function iso(ts?: { toDate(): Date }): string {
  return ts ? ts.toDate().toISOString() : "";
}

export interface DeletionStatus {
  pending: boolean;
  eraseAfter: string;
}

export async function getDeletionStatus(): Promise<DeletionStatus> {
  const res = await makeClients(getToken()).privacy.getAccountDeletionStatus(
    {},
  );
  return { pending: res.pending, eraseAfter: iso(res.eraseAfter) };
}

export async function requestDeletion(
  password: string,
): Promise<{ eraseAfter: string }> {
  const res = await makeClients(getToken()).privacy.requestAccountDeletion({
    password,
  });
  return { eraseAfter: iso(res.eraseAfter) };
}

export async function cancelDeletion(): Promise<void> {
  await makeClients(getToken()).privacy.cancelAccountDeletion({});
}

export interface ExportedData {
  content: string;
  contentType: string;
  generatedAt: string;
}

export async function exportMyData(): Promise<ExportedData> {
  const res = await makeClients(getToken()).privacy.exportMyData({});
  return {
    content: new TextDecoder().decode(res.data),
    contentType: res.contentType,
    generatedAt: iso(res.generatedAt),
  };
}
