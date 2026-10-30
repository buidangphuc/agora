import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { UserStatus as Pb } from "@/generated/platform/identity/v1/identity_pb.js";

import { makeClients } from "./client.js";
import { listUsers, setUserStatus } from "./users.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/users.ts):
//   listUsers({ query, status? }, cursor) =>
//     { status: "ok"; users: AdminUserView[]; nextCursor: string }
//   | { status: "unauthenticated" | "forbidden" | "invalid" | "unavailable" }
//   AdminUserView = { id, username: string; roles: string[];
//     status: "active"|"restricted"|"suspended"|"banned";
//     statusReason: string; statusUntil: string /* ISO-8601 UTC, "" if none */ }
//   setUserStatus({ userId, status, reasonCode, note, until? }) =>
//     { status: "ok"; user: AdminUserView; sessionsRevoked: number }
//   | { status: "unauthenticated" | "forbidden" | "invalid"
//       | "failed_precondition" | "not_found" | "unavailable" }
// Results never carry err.message.

function stubAdmin(listUsersRpc = vi.fn(), setUserStatusRpc = vi.fn()) {
  const accountAdmin = {
    listUsers: listUsersRpc,
    setUserStatus: setUserStatusRpc,
  };
  vi.mocked(makeClients).mockReturnValue({ accountAdmin } as never);
  return accountAdmin;
}

const okList = () =>
  vi.fn().mockResolvedValue({ users: [], page: { nextCursor: "" } });
const okSet = () =>
  vi.fn().mockResolvedValue({
    user: { id: "u1", username: "an01", roles: ["buyer"], status: Pb.BANNED },
    sessionsRevoked: 3,
  });

const input = {
  userId: "u1",
  status: "banned" as const,
  reasonCode: "fraud",
  note: "n",
};

beforeEach(() => vi.clearAllMocks());

describe("listUsers gateway wrapper", () => {
  it("forwards query, status, cursor and page size 20 (Users UI: Search is forwarded as typed fields)", async () => {
    const a = stubAdmin(okList());
    await listUsers({ query: "an01", status: "suspended" }, "c2");
    const [req, opts] = a.listUsers.mock.calls[0];
    expect(req.query).toBe("an01");
    expect(req.status).toBe(Pb.SUSPENDED);
    expect(req.page).toMatchObject({ cursor: "c2", pageSize: 20 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
  });

  it("sends UNSPECIFIED status when no status filter (Users UI: Search is forwarded as typed fields)", async () => {
    const a = stubAdmin(okList());
    await listUsers({ query: "" }, "");
    expect(a.listUsers.mock.calls[0][0].status ?? Pb.UNSPECIFIED).toBe(
      Pb.UNSPECIFIED,
    );
  });

  it.each([
    ["active", Pb.ACTIVE],
    ["restricted", Pb.RESTRICTED],
    ["banned", Pb.BANNED],
  ] as const)(
    "maps status %s (Users UI: Search is forwarded as typed fields)",
    async (s, pb) => {
      const a = stubAdmin(okList());
      await listUsers({ query: "", status: s }, "");
      expect(a.listUsers.mock.calls[0][0].status).toBe(pb);
    },
  );

  it("maps users and next cursor (Users UI: Search is forwarded as typed fields)", async () => {
    stubAdmin(
      vi.fn().mockResolvedValue({
        users: [
          {
            id: "u1",
            username: "an01",
            roles: ["seller", "buyer"],
            status: Pb.SUSPENDED,
            statusReason: "fraud",
            statusUntil: Timestamp.fromDate(new Date("2026-10-20T10:00:00Z")),
          },
          { id: "u2", username: "b", roles: [], status: Pb.ACTIVE },
        ],
        page: { nextCursor: "c3" },
      }),
    );
    const r = await listUsers({ query: "" }, "");
    expect(r).toEqual({
      status: "ok",
      nextCursor: "c3",
      users: [
        {
          id: "u1",
          username: "an01",
          roles: ["seller", "buyer"],
          status: "suspended",
          statusReason: "fraud",
          statusUntil: "2026-10-20T10:00:00.000Z",
        },
        {
          id: "u2",
          username: "b",
          roles: [],
          status: "active",
          statusReason: "",
          statusUntil: "",
        },
      ],
    });
  });

  it.each([
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.InvalidArgument, "invalid"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "unavailable"],
  ])(
    "maps unavailable and timeout: code %s -> %s (Users UI: Identity unavailable)",
    async (code, status) => {
      stubAdmin(vi.fn().mockRejectedValue(new ConnectError("boom", code)));
      expect(await listUsers({ query: "" }, "")).toEqual({ status });
    },
  );

  it("maps a non-Connect error to unavailable without a message (Users UI: Identity unavailable)", async () => {
    stubAdmin(vi.fn().mockRejectedValue(new Error("10.0.0.5:9000 refused")));
    const r = await listUsers({ query: "" }, "");
    expect(r).toEqual({ status: "unavailable" });
    expect(JSON.stringify(r)).not.toContain("10.0.0.5");
  });
});

describe("setUserStatus gateway wrapper", () => {
  it("sends a 10 second timeout and the typed fields (Users UI: Server refuses a self or admin target)", async () => {
    const a = stubAdmin(vi.fn(), okSet());
    await setUserStatus(input);
    const [req, opts] = a.setUserStatus.mock.calls[0];
    expect(req).toMatchObject({
      userId: "u1",
      status: Pb.BANNED,
      reasonCode: "fraud",
      note: "n",
    });
    expect(opts).toMatchObject({ timeoutMs: 10000 });
  });

  it("sends until only for SUSPENDED (Users UI: Audit service down leaves the user unchanged)", async () => {
    const a = stubAdmin(vi.fn(), okSet());
    const until = new Date("2026-10-20T10:00:00Z");
    await setUserStatus({ ...input, status: "suspended", until });
    await setUserStatus({ ...input, status: "banned", until });
    await setUserStatus({ ...input, status: "restricted", until });
    await setUserStatus({ ...input, status: "active", until });
    const [susp, ...others] = a.setUserStatus.mock.calls.map((c) => c[0]);
    expect(susp.status).toBe(Pb.SUSPENDED);
    expect(susp.until?.toDate().toISOString()).toBe("2026-10-20T10:00:00.000Z");
    for (const o of others) expect(o.until).toBeUndefined();
  });

  it("returns the user and revoked session count on success (Users UI: Server refuses a self or admin target)", async () => {
    stubAdmin(vi.fn(), okSet());
    const r = await setUserStatus(input);
    expect(r).toMatchObject({
      status: "ok",
      sessionsRevoked: 3,
      user: { id: "u1", username: "an01", status: "banned" },
    });
  });

  it.each([
    [Code.Unauthenticated, "unauthenticated"],
    [Code.PermissionDenied, "forbidden"],
    [Code.InvalidArgument, "invalid"],
    [Code.NotFound, "not_found"],
  ])(
    "maps code %s -> %s (Users UI: Server refuses a self or admin target)",
    async (code, status) => {
      stubAdmin(
        vi.fn(),
        vi.fn().mockRejectedValue(new ConnectError("x", code)),
      );
      expect(await setUserStatus(input)).toEqual({ status });
    },
  );

  it("maps failed precondition (Users UI: Server refuses a self or admin target)", async () => {
    stubAdmin(
      vi.fn(),
      vi
        .fn()
        .mockRejectedValue(new ConnectError("self", Code.FailedPrecondition)),
    );
    expect(await setUserStatus(input)).toEqual({
      status: "failed_precondition",
    });
  });

  it.each([Code.Unavailable, Code.DeadlineExceeded, Code.Internal])(
    "maps unavailable on set status: code %s (Users UI: Audit service down leaves the user unchanged)",
    async (code) => {
      stubAdmin(
        vi.fn(),
        vi
          .fn()
          .mockRejectedValue(new ConnectError("audit at 10.1.1.1 down", code)),
      );
      const r = await setUserStatus(input);
      expect(r).toEqual({ status: "unavailable" });
      expect(JSON.stringify(r)).not.toContain("10.1.1.1");
    },
  );

  it("maps a non-Connect error to unavailable (Users UI: Audit service down leaves the user unchanged)", async () => {
    stubAdmin(
      vi.fn(),
      vi.fn().mockRejectedValue(new TypeError("fetch failed")),
    );
    expect(await setUserStatus(input)).toEqual({ status: "unavailable" });
  });
});
