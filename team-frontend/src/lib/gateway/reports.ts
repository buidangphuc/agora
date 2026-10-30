/**
 * Report calls over the Gateway's EngagementService.
 * server-only; never throws and never returns an error message (it could carry
 * upstream addresses).
 */
import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";

import {
  type Report,
  ReportStatus,
  ReportTargetType,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export type ReportStatusName = "open" | "actioned" | "dismissed";

export type ErrStatus =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "failed_precondition"
  | "invalid"
  | "resource_exhausted"
  | "unavailable";

export interface ReportView {
  id: string;
  listingId: string;
  reasonCode: string;
  note: string;
  reporterId: string;
  status: ReportStatusName;
  /** ISO-8601 UTC. */
  createdAt: string;
  resolutionNote: string;
}

export type CreateReportResult =
  | { status: "ok"; report: ReportView }
  | { status: ErrStatus };

const TIMEOUT_MS = 5000;

function statusName(s: ReportStatus): ReportStatusName {
  if (s === ReportStatus.ACTIONED) return "actioned";
  if (s === ReportStatus.DISMISSED) return "dismissed";
  return "open";
}

function toView(r: Report): ReportView {
  return {
    id: r.id,
    listingId: r.targetId,
    reasonCode: r.reasonCode,
    note: r.note,
    reporterId: r.reporterId,
    status: statusName(r.status),
    createdAt: r.createdAt?.toDate().toISOString() ?? "",
    resolutionNote: r.resolutionNote,
  };
}

function toErr(err: unknown): { status: ErrStatus } {
  if (err instanceof ConnectError) {
    switch (err.code) {
      case Code.Unauthenticated:
        return { status: "unauthenticated" };
      case Code.PermissionDenied:
        return { status: "forbidden" };
      case Code.NotFound:
        return { status: "not_found" };
      case Code.FailedPrecondition:
        return { status: "failed_precondition" };
      case Code.InvalidArgument:
        return { status: "invalid" };
      case Code.ResourceExhausted:
        return { status: "resource_exhausted" };
    }
  }
  return { status: "unavailable" };
}

export async function createReport(
  listingId: string,
  reasonCode: string,
  note: string,
): Promise<CreateReportResult> {
  try {
    const { engagement } = makeClients(getToken());
    const res = await engagement.createReport(
      {
        targetType: ReportTargetType.LISTING,
        targetId: listingId,
        reasonCode,
        note,
      },
      { timeoutMs: TIMEOUT_MS },
    );
    if (!res.report) return { status: "unavailable" };
    return { status: "ok", report: toView(res.report) };
  } catch (err) {
    return toErr(err);
  }
}
