/**
 * Admin user listing and enforcement over the Gateway's AccountAdminService.
 * server-only; never throws and never returns an error message (it could carry
 * upstream addresses). The gateway enforces the `admin` scope.
 */
import "server-only";

import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";

import { UserStatus as Pb } from "@/generated/platform/identity/v1/identity_pb.js";

import { makeClients } from "./client.js";
import { getToken } from "./session.js";

export type UserStatusName = "active" | "restricted" | "suspended" | "banned";

export interface AdminUserView {
  id: string;
  username: string;
  roles: string[];
  status: UserStatusName;
  statusReason: string;
  /** ISO-8601 UTC, "" if none. */
  statusUntil: string;
}

export type ListUsersResult =
  | { status: "ok"; users: AdminUserView[]; nextCursor: string }
  | { status: "unauthenticated" | "forbidden" | "invalid" | "unavailable" };

export type SetUserStatusResult =
  | { status: "ok"; user: AdminUserView; sessionsRevoked: number }
  | {
      status:
        | "unauthenticated"
        | "forbidden"
        | "invalid"
        | "failed_precondition"
        | "not_found"
        | "unavailable";
    };

const PAGE_SIZE = 20;
const LIST_TIMEOUT_MS = 5000;
const SET_TIMEOUT_MS = 10000;

const TO_PB: Record<UserStatusName, Pb> = {
  active: Pb.ACTIVE,
  restricted: Pb.RESTRICTED,
  suspended: Pb.SUSPENDED,
  banned: Pb.BANNED,
};

function fromPb(s: Pb): UserStatusName {
  switch (s) {
    case Pb.RESTRICTED:
      return "restricted";
    case Pb.SUSPENDED:
      return "suspended";
    case Pb.BANNED:
      return "banned";
    default:
      return "active";
  }
}

function toView(u: {
  id: string;
  username: string;
  roles: string[];
  status: Pb;
  statusReason?: string;
  statusUntil?: Timestamp;
}): AdminUserView {
  return {
    id: u.id,
    username: u.username,
    roles: [...u.roles],
    status: fromPb(u.status),
    statusReason: u.statusReason ?? "",
    statusUntil: u.statusUntil?.toDate().toISOString() ?? "",
  };
}

export async function listUsers(
  filters: { query: string; status?: UserStatusName },
  cursor: string,
): Promise<ListUsersResult> {
  try {
    const { accountAdmin } = makeClients(getToken());
    const res = await accountAdmin.listUsers(
      {
        query: filters.query,
        status: filters.status ? TO_PB[filters.status] : Pb.UNSPECIFIED,
        page: { cursor, pageSize: PAGE_SIZE },
      },
      { timeoutMs: LIST_TIMEOUT_MS },
    );
    return {
      status: "ok",
      nextCursor: res.page?.nextCursor ?? "",
      users: res.users.map(toView),
    };
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated)
        return { status: "unauthenticated" };
      if (err.code === Code.PermissionDenied) return { status: "forbidden" };
      if (err.code === Code.InvalidArgument) return { status: "invalid" };
    }
    return { status: "unavailable" };
  }
}

export async function setUserStatus(input: {
  userId: string;
  status: UserStatusName;
  reasonCode: string;
  note: string;
  until?: Date;
}): Promise<SetUserStatusResult> {
  try {
    const { accountAdmin } = makeClients(getToken());
    const res = await accountAdmin.setUserStatus(
      {
        userId: input.userId,
        status: TO_PB[input.status],
        reasonCode: input.reasonCode,
        note: input.note,
        until:
          input.status === "suspended" && input.until
            ? Timestamp.fromDate(input.until)
            : undefined,
      },
      { timeoutMs: SET_TIMEOUT_MS },
    );
    if (!res.user) return { status: "unavailable" };
    return {
      status: "ok",
      user: toView(res.user),
      sessionsRevoked: res.sessionsRevoked,
    };
  } catch (err) {
    if (err instanceof ConnectError) {
      if (err.code === Code.Unauthenticated)
        return { status: "unauthenticated" };
      if (err.code === Code.PermissionDenied) return { status: "forbidden" };
      if (err.code === Code.InvalidArgument) return { status: "invalid" };
      if (err.code === Code.FailedPrecondition)
        return { status: "failed_precondition" };
      if (err.code === Code.NotFound) return { status: "not_found" };
    }
    return { status: "unavailable" };
  }
}
