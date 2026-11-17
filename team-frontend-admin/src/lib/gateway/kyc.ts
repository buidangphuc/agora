import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import { PageRequest } from "@/generated/platform/common/v1/common_pb.js";
import {
  GetKycDocumentUrlRequest,
  ListKycSubmissionsRequest,
  ReviewKycRequest,
  VerificationStatus,
} from "@/generated/platform/verification/v1/verification_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

const TIMEOUT_MS = 5000;
const PAGE_SIZE = 20;

export type KycStatusName = "PENDING" | "VERIFIED" | "REJECTED";

export interface KycSubmissionView {
  id: string;
  userId: string;
  docType: string;
  status: KycStatusName;
  documentCount: number;
  reasonCode: string;
  createdAt: string;
  reviewedAt: string;
}

export type KycFailure =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "failed_precondition"
  | "invalid"
  | "unavailable";

export type ListKycResult =
  | { status: "ok"; items: KycSubmissionView[]; nextCursor: string }
  | { status: "unauthenticated" | "forbidden" | "unavailable" };

export type KycDocumentUrlResult =
  | { status: "ok"; url: string; expiresAt: string }
  | { status: KycFailure };

export type ReviewKycResult = { status: "ok" } | { status: KycFailure };

const STATUS_ENUM: Record<KycStatusName, VerificationStatus> = {
  PENDING: VerificationStatus.PENDING,
  VERIFIED: VerificationStatus.VERIFIED,
  REJECTED: VerificationStatus.REJECTED,
};

function statusName(s: VerificationStatus): KycStatusName {
  if (s === VerificationStatus.VERIFIED) return "VERIFIED";
  if (s === VerificationStatus.REJECTED) return "REJECTED";
  return "PENDING";
}

function failure(err: unknown): KycFailure {
  if (!(err instanceof ConnectError)) return "unavailable";
  switch (err.code) {
    case Code.Unauthenticated:
      return "unauthenticated";
    case Code.PermissionDenied:
      return "forbidden";
    case Code.NotFound:
      return "not_found";
    case Code.FailedPrecondition:
      return "failed_precondition";
    case Code.InvalidArgument:
      return "invalid";
    default:
      return "unavailable";
  }
}

export async function listKycSubmissions(
  status: KycStatusName,
  cursor: string,
): Promise<ListKycResult> {
  try {
    const res = await makeClients(getToken()).verification.listKycSubmissions(
      new ListKycSubmissionsRequest({
        status: STATUS_ENUM[status],
        page: new PageRequest({ cursor, pageSize: PAGE_SIZE }),
      }),
      { timeoutMs: TIMEOUT_MS },
    );
    return {
      status: "ok",
      items: res.submissions.map((k) => ({
        id: k.id,
        userId: k.userId,
        docType: k.docType,
        status: statusName(k.status),
        documentCount: k.documentCount,
        reasonCode: k.reasonCode,
        createdAt: k.createdAt ? k.createdAt.toDate().toISOString() : "",
        reviewedAt: k.reviewedAt ? k.reviewedAt.toDate().toISOString() : "",
      })),
      nextCursor: res.page?.nextCursor ?? "",
    };
  } catch (err) {
    const f = failure(err);
    return {
      status: f === "unauthenticated" || f === "forbidden" ? f : "unavailable",
    };
  }
}

export async function getKycDocumentUrl(
  id: string,
  index: number,
): Promise<KycDocumentUrlResult> {
  try {
    const res = await makeClients(getToken()).verification.getKycDocumentUrl(
      new GetKycDocumentUrlRequest({ submissionId: id, index }),
      { timeoutMs: TIMEOUT_MS },
    );
    return {
      status: "ok",
      url: res.url,
      expiresAt: res.expiresAt ? res.expiresAt.toDate().toISOString() : "",
    };
  } catch (err) {
    return { status: failure(err) };
  }
}

export async function reviewKyc(
  id: string,
  decision: "approve" | "reject",
  reasonCode: string,
): Promise<ReviewKycResult> {
  try {
    await makeClients(getToken()).verification.reviewKyc(
      new ReviewKycRequest({ id, decision, reasonCode }),
      { timeoutMs: TIMEOUT_MS },
    );
    return { status: "ok" };
  } catch (err) {
    return { status: failure(err) };
  }
}
