import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { queryAuditLog } from "./audit.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/audit.ts):
//   queryAuditLog(filters, cursor) =>
//     { status: "ok"; events: AuditEventView[]; nextCursor: string }
//   | { status: "unauthenticated" | "forbidden" | "invalid_range" | "unavailable" }
//   AuditEventView = { id, actorId, action, targetType, targetId: string;
//                      createdAt: string /* ISO-8601 UTC */;
//                      metadata: Record<string, string> }
//   filters = { targetType?, targetId?, actorId?, action?: string;
//               createdAfter?, createdBefore?: Date }

function stubAudit(rpc: ReturnType<typeof vi.fn>) {
  const audit = { queryAuditLog: rpc };
  vi.mocked(makeClients).mockReturnValue({ audit } as never);
  return audit;
}

beforeEach(() => vi.clearAllMocks());

describe("queryAuditLog gateway wrapper", () => {
  it("sends page size 20 and the cursor (Page size is 20)", async () => {
    const audit = stubAudit(
      vi.fn().mockResolvedValue({ events: [], page: { nextCursor: "" } }),
    );
    await queryAuditLog({}, "");
    await queryAuditLog({}, "40");
    const [first] = audit.queryAuditLog.mock.calls[0];
    const [second] = audit.queryAuditLog.mock.calls[1];
    expect(first.page).toMatchObject({ cursor: "", pageSize: 20 });
    expect(second.page).toMatchObject({ cursor: "40", pageSize: 20 });
  });

  it("uses a 5 second timeout (Page size is 20)", async () => {
    const audit = stubAudit(
      vi.fn().mockResolvedValue({ events: [], page: { nextCursor: "" } }),
    );
    await queryAuditLog({}, "");
    expect(audit.queryAuditLog.mock.calls[0][1]).toMatchObject({
      timeoutMs: 5000,
    });
  });

  it("forwards filters as typed request fields (All filters are forwarded)", async () => {
    const audit = stubAudit(
      vi.fn().mockResolvedValue({ events: [], page: { nextCursor: "" } }),
    );
    await queryAuditLog(
      {
        targetType: "order",
        targetId: "o-1",
        actorId: "a-1",
        action: "order.force_fail",
        createdAfter: new Date("2026-09-30T17:00:00Z"),
        createdBefore: new Date("2026-10-02T17:00:00Z"),
      },
      "",
    );
    const [req] = audit.queryAuditLog.mock.calls[0];
    expect(req.targetType).toBe("order");
    expect(req.targetId).toBe("o-1");
    expect(req.actorId).toBe("a-1");
    expect(req.action).toBe("order.force_fail");
    expect(req.createdAfter.toDate().toISOString()).toBe(
      "2026-09-30T17:00:00.000Z",
    );
    expect(req.createdBefore.toDate().toISOString()).toBe(
      "2026-10-02T17:00:00.000Z",
    );
  });

  it("omits unset filters from the request (Empty and whitespace filters are omitted)", async () => {
    const audit = stubAudit(
      vi.fn().mockResolvedValue({ events: [], page: { nextCursor: "" } }),
    );
    await queryAuditLog({}, "");
    const [req] = audit.queryAuditLog.mock.calls[0];
    expect(req.targetId).toBeUndefined();
    expect(req.action).toBeUndefined();
    expect(req.createdAfter).toBeUndefined();
    expect(req.createdBefore).toBeUndefined();
    expect(req.actorId || "").toBe("");
    expect(req.targetType || "").toBe("");
  });

  it("maps events to view types with ISO time and metadata (Rows show the five columns)", async () => {
    stubAudit(
      vi.fn().mockResolvedValue({
        events: [
          {
            id: "e-1",
            actorId: "u-1",
            action: "kyc.review",
            targetType: "kyc_submission",
            targetId: "s-9",
            metadata: { user_id: "u-7", decision: "approve" },
            createdAt: Timestamp.fromDate(new Date("2026-10-05T17:00:00Z")),
          },
        ],
        page: { nextCursor: "20" },
      }),
    );
    const res = await queryAuditLog({}, "");
    expect(res).toEqual({
      status: "ok",
      nextCursor: "20",
      events: [
        {
          id: "e-1",
          actorId: "u-1",
          action: "kyc.review",
          targetType: "kyc_submission",
          targetId: "s-9",
          createdAt: "2026-10-05T17:00:00.000Z",
          metadata: { user_id: "u-7", decision: "approve" },
        },
      ],
    });
  });

  it("returns an empty next cursor when the page has none (Last page has no next link)", async () => {
    stubAudit(vi.fn().mockResolvedValue({ events: [], page: undefined }));
    const res = await queryAuditLog({}, "");
    expect(res).toEqual({ status: "ok", events: [], nextCursor: "" });
  });

  it("maps InvalidArgument to invalid_range (End date before start date)", async () => {
    stubAudit(
      vi
        .fn()
        .mockRejectedValue(
          new ConnectError(
            "created_after > created_before",
            Code.InvalidArgument,
          ),
        ),
    );
    await expect(queryAuditLog({}, "")).resolves.toEqual({
      status: "invalid_range",
    });
  });

  it("maps Unauthenticated and PermissionDenied (Expired session is sent to login; Gateway refusal is shown as the same 403)", async () => {
    stubAudit(
      vi.fn().mockRejectedValue(new ConnectError("x", Code.Unauthenticated)),
    );
    await expect(queryAuditLog({}, "")).resolves.toEqual({
      status: "unauthenticated",
    });
    stubAudit(
      vi.fn().mockRejectedValue(new ConnectError("x", Code.PermissionDenied)),
    );
    await expect(queryAuditLog({}, "")).resolves.toEqual({
      status: "forbidden",
    });
  });

  it("maps Unavailable and unknown errors to unavailable without leaking the message (team-audit unavailable)", async () => {
    stubAudit(
      vi
        .fn()
        .mockRejectedValue(
          new ConnectError("dial tcp 10.0.0.7 refused", Code.Unavailable),
        ),
    );
    const a = await queryAuditLog({}, "");
    expect(a).toEqual({ status: "unavailable" });
    stubAudit(vi.fn().mockRejectedValue(new Error("boom secret")));
    const b = await queryAuditLog({}, "");
    expect(b).toEqual({ status: "unavailable" });
    stubAudit(
      vi
        .fn()
        .mockRejectedValue(new ConnectError("slow", Code.DeadlineExceeded)),
    );
    const c = await queryAuditLog({}, "");
    expect(c).toEqual({ status: "unavailable" });
    expect(JSON.stringify([a, b, c])).not.toMatch(/10\.0\.0\.7|secret|slow/);
  });
});
