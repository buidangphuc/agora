import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import {
  CreateDisputeRequest,
  type Dispute,
  DisputeRuling,
  DisputeStatus,
  ListOrderDisputesRequest,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

const TIMEOUT_MS = 5000;

export type DisputeStatusName =
  | "OPEN"
  | "INVESTIGATING"
  | "RESOLVED"
  | "REJECTED";
export type DisputeRulingName = "" | "BUYER_FAVOUR" | "SELLER_FAVOUR";

export interface ViewDispute {
  id: string;
  orderId: string;
  claimantId: string;
  defendantId: string;
  reason: string;
  resolution: string;
  returnId: string;
  evidenceUrls: string[];
  status: DisputeStatusName;
  statusText: string;
  ruling: DisputeRulingName;
  rulingText: string;
  createdAt: string;
}

const STATUS_TEXT: Record<DisputeStatusName, string> = {
  OPEN: "Đang chờ xử lý",
  INVESTIGATING: "Đang điều tra",
  RESOLVED: "Đã giải quyết",
  REJECTED: "Đã từ chối",
};

const RULING_TEXT: Record<"BUYER_FAVOUR" | "SELLER_FAVOUR", string> = {
  BUYER_FAVOUR: "Người mua thắng",
  SELLER_FAVOUR: "Người bán thắng",
};

function statusName(s: DisputeStatus): DisputeStatusName {
  switch (s) {
    case DisputeStatus.INVESTIGATING:
      return "INVESTIGATING";
    case DisputeStatus.RESOLVED:
      return "RESOLVED";
    case DisputeStatus.REJECTED:
      return "REJECTED";
    default:
      return "OPEN";
  }
}

function rulingName(r: DisputeRuling): DisputeRulingName {
  if (r === DisputeRuling.BUYER_FAVOUR) return "BUYER_FAVOUR";
  if (r === DisputeRuling.SELLER_FAVOUR) return "SELLER_FAVOUR";
  return "";
}

function mapDispute(d: Dispute): ViewDispute {
  const status = statusName(d.status);
  const ruling = rulingName(d.ruling);
  return {
    id: d.id,
    orderId: d.orderId,
    claimantId: d.claimantId,
    defendantId: d.defendantId,
    reason: d.reason,
    resolution: d.resolution,
    returnId: d.returnId,
    evidenceUrls: d.evidenceUrls ?? [],
    status,
    statusText: STATUS_TEXT[status],
    ruling,
    rulingText: ruling === "" ? "" : RULING_TEXT[ruling],
    createdAt: d.createdAt ? d.createdAt.toDate().toISOString() : "",
  };
}

function codeOf(err: unknown): Code | undefined {
  return err instanceof ConnectError ? err.code : undefined;
}

export type CreateDisputeResult =
  | { status: "ok"; dispute: ViewDispute }
  | {
      status:
        | "failed_precondition"
        | "already_exists"
        | "permission_denied"
        | "not_found"
        | "invalid_argument"
        | "unauthenticated"
        | "unavailable";
    };

export async function createDispute(input: {
  orderId: string;
  defendantId: string;
  returnId: string;
  reason: string;
}): Promise<CreateDisputeResult> {
  try {
    const res = await makeClients(getToken()).engagement.createDispute(
      new CreateDisputeRequest({
        orderId: input.orderId,
        defendantId: input.defendantId,
        returnId: input.returnId,
        reason: input.reason,
      }),
      { timeoutMs: TIMEOUT_MS },
    );
    if (!res.dispute) return { status: "unavailable" };
    return { status: "ok", dispute: mapDispute(res.dispute) };
  } catch (err) {
    switch (codeOf(err)) {
      case Code.FailedPrecondition:
        return { status: "failed_precondition" };
      case Code.AlreadyExists:
        return { status: "already_exists" };
      case Code.PermissionDenied:
        return { status: "permission_denied" };
      case Code.NotFound:
        return { status: "not_found" };
      case Code.InvalidArgument:
        return { status: "invalid_argument" };
      case Code.Unauthenticated:
        return { status: "unauthenticated" };
      default:
        return { status: "unavailable" };
    }
  }
}

export type ListOrderDisputesResult =
  | { status: "ok"; disputes: ViewDispute[] }
  | { status: "unavailable" };

export async function listOrderDisputes(
  orderId: string,
): Promise<ListOrderDisputesResult> {
  try {
    const res = await makeClients(getToken()).engagement.listOrderDisputes(
      new ListOrderDisputesRequest({ orderId }),
      { timeoutMs: TIMEOUT_MS },
    );
    return { status: "ok", disputes: res.disputes.map(mapDispute) };
  } catch {
    return { status: "unavailable" };
  }
}
