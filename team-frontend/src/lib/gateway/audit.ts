/**
 * Admin audit-log query over the Gateway's AuditService.QueryAuditLog.
 * server-only; never throws and never returns an error message (it could carry
 * upstream addresses). The gateway enforces the `admin` scope.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import type { AuditFilters } from "@/features/admin/auditFilters";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export interface AuditEventView {
  id: string;
  actorId: string;
  action: string;
  targetType: string;
  targetId: string;
  /** ISO-8601 UTC. */
  createdAt: string;
  metadata: Record<string, string>;
}

export type AuditLogResult =
  | { status: "ok"; events: AuditEventView[]; nextCursor: string }
  | { status: "unauthenticated" }
  | { status: "forbidden" }
  | { status: "invalid_range" }
  | { status: "unavailable" };

const PAGE_SIZE = 20;
const TIMEOUT_MS = 5000;

export async function queryAuditLog(
  filters: AuditFilters,
  cursor: string,
): Promise<AuditLogResult> {
  try {
    const { audit } = makeClients(getToken());
    const res = await audit.queryAuditLog(
      {
        targetType: filters.targetType,
        targetId: filters.targetId,
        actorId: filters.actorId,
        action: filters.action,
        createdAfter: filters.createdAfter
          ? Timestamp.fromDate(filters.createdAfter)
          : undefined,
        createdBefore: filters.createdBefore
          ? Timestamp.fromDate(filters.createdBefore)
          : undefined,
        page: { cursor, pageSize: PAGE_SIZE },
      },
      { timeoutMs: TIMEOUT_MS },
    );
    return {
      status: "ok",
      nextCursor: res.page?.nextCursor ?? "",
      events: res.events.map((e) => ({
        id: e.id,
        actorId: e.actorId,
        action: e.action,
        targetType: e.targetType,
        targetId: e.targetId,
        createdAt: e.createdAt?.toDate().toISOString() ?? "",
        metadata: { ...e.metadata },
      })),
    };
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated)
        return { status: "unauthenticated" };
      if (err.code === Code.PermissionDenied) return { status: "forbidden" };
      if (err.code === Code.InvalidArgument) return { status: "invalid_range" };
    }
    return { status: "unavailable" };
  }
}
