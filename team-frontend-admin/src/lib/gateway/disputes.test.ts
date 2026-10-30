import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  DisputeRuling,
  DisputeStatus,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import {
  createDispute,
  getDispute,
  listDisputes,
  listOrderDisputes,
  resolveDispute,
} from "./disputes.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/disputes.ts):
//   ViewDispute = { id, orderId, claimantId, defendantId, reason, resolution,
//                   returnId: string; evidenceUrls: string[];
//                   status: "OPEN"|"INVESTIGATING"|"RESOLVED"|"REJECTED";
//                   statusText: string /* vi copy */;
//                   ruling: ""|"BUYER_FAVOUR"|"SELLER_FAVOUR";
//                   rulingText: string /* "" when no ruling */;
//                   createdAt: string /* ISO-8601 UTC, "" if unset */ }
//   createDispute({orderId, defendantId, returnId, reason}) =>
//     {status:"ok", dispute} | {status: <lowercase ConnectError code name>}
//       for failed_precondition|already_exists|permission_denied|not_found|
//       invalid_argument|unauthenticated; any other failure or timeout => "unavailable"
//   listOrderDisputes(orderId) => {status:"ok", disputes} | {status:"unavailable"}
//   listDisputes(status: string|undefined, cursor) =>
//     {status:"ok", disputes, nextCursor} | {status:"unauthenticated"|"forbidden"|"unavailable"}
//   getDispute(id) => {status:"ok", dispute} | {status:"not_found"|"unauthenticated"|"forbidden"|"unavailable"}
//   resolveDispute({disputeId, status:"INVESTIGATING"|"RESOLVED"|"REJECTED",
//                   resolution, ruling?:"BUYER_FAVOUR"|"SELLER_FAVOUR"}) =>
//     {status:"ok", dispute} | {status:"failed_precondition"|"permission_denied"|
//       "not_found"|"invalid_argument"|"unauthenticated"|"unavailable"|"timeout"}
//   No result ever carries err.message.

function pbDispute(over: Record<string, unknown> = {}) {
  return {
    id: "d-1",
    orderId: "o-1",
    claimantId: "u-1",
    defendantId: "s-1",
    reason: "hang loi",
    evidenceUrls: ["https://x.test/a.jpg"],
    status: DisputeStatus.OPEN,
    resolution: "",
    createdAt: Timestamp.fromDate(new Date("2026-10-05T17:00:00Z")),
    returnId: "r-1",
    ruling: DisputeRuling.UNSPECIFIED,
    ...over,
  };
}

function stub(methods: Record<string, ReturnType<typeof vi.fn>>) {
  vi.mocked(makeClients).mockReturnValue({ engagement: methods } as never);
  return methods;
}

const SECRET = "dial tcp 10.0.0.7:9090 secret-upstream";
const reject = (code: Code) =>
  vi.fn().mockRejectedValue(new ConnectError(SECRET, code));

beforeEach(() => vi.clearAllMocks());

describe("listDisputes", () => {
  it("listDisputes defaults to OPEN and page size 20 (Default list is OPEN, 20 per page)", async () => {
    const m = stub({
      listDisputes: vi.fn().mockResolvedValue({ disputes: [], page: {} }),
    });
    await listDisputes(undefined, "");
    expect(m.listDisputes).toHaveBeenCalledTimes(1);
    const [req, opts] = m.listDisputes.mock.calls[0];
    expect(req.status).toBe(DisputeStatus.OPEN);
    expect(req.page).toMatchObject({ cursor: "", pageSize: 20 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
  });

  it("forwards a known status and the cursor (Default list is OPEN, 20 per page)", async () => {
    const m = stub({
      listDisputes: vi.fn().mockResolvedValue({ disputes: [], page: {} }),
    });
    await listDisputes("INVESTIGATING", "20");
    const [req] = m.listDisputes.mock.calls[0];
    expect(req.status).toBe(DisputeStatus.INVESTIGATING);
    expect(req.page).toMatchObject({ cursor: "20", pageSize: 20 });
  });

  it.each(["bogus", "", "open", "UNSPECIFIED"])(
    "ignores an unknown status %j (Unknown status falls back to OPEN)",
    async (bad) => {
      const m = stub({
        listDisputes: vi.fn().mockResolvedValue({ disputes: [], page: {} }),
      });
      await listDisputes(bad, "");
      expect(m.listDisputes.mock.calls[0][0].status).toBe(DisputeStatus.OPEN);
    },
  );

  it("maps rows and the next cursor to view types (Default list is OPEN, 20 per page)", async () => {
    stub({
      listDisputes: vi.fn().mockResolvedValue({
        disputes: [
          pbDispute(),
          pbDispute({
            id: "d-2",
            status: DisputeStatus.RESOLVED,
            ruling: DisputeRuling.BUYER_FAVOUR,
            resolution: "ok",
          }),
        ],
        page: { nextCursor: "20" },
      }),
    });
    const res = await listDisputes("OPEN", "");
    expect(res.status).toBe("ok");
    if (res.status !== "ok") return;
    expect(res.nextCursor).toBe("20");
    expect(res.disputes).toHaveLength(2);
    expect(res.disputes[0]).toMatchObject({
      id: "d-1",
      orderId: "o-1",
      claimantId: "u-1",
      defendantId: "s-1",
      reason: "hang loi",
      returnId: "r-1",
      evidenceUrls: ["https://x.test/a.jpg"],
      status: "OPEN",
      statusText: "Đang chờ xử lý",
      ruling: "",
      rulingText: "",
      createdAt: "2026-10-05T17:00:00.000Z",
    });
    expect(res.disputes[1]).toMatchObject({
      status: "RESOLVED",
      statusText: "Đã giải quyết",
      ruling: "BUYER_FAVOUR",
      rulingText: "Người mua thắng",
      resolution: "ok",
    });
  });

  it("listDisputes maps PermissionDenied to forbidden (Non-admin token is denied the list)", async () => {
    stub({ listDisputes: reject(Code.PermissionDenied) });
    const res = await listDisputes("OPEN", "");
    expect(res).toEqual({ status: "forbidden" });
    expect(JSON.stringify(res)).not.toContain("secret-upstream");
  });

  it("maps Unauthenticated to unauthenticated (Gateway refusal and expired session)", async () => {
    stub({ listDisputes: reject(Code.Unauthenticated) });
    expect(await listDisputes("OPEN", "")).toEqual({
      status: "unauthenticated",
    });
  });

  it("listDisputes maps UNAVAILABLE to an error result (Load failure)", async () => {
    stub({ listDisputes: reject(Code.Unavailable) });
    const res = await listDisputes("OPEN", "");
    expect(res).toEqual({ status: "unavailable" });
    expect(JSON.stringify(res)).not.toContain("secret-upstream");
  });

  it("maps a non-Connect throw to unavailable (Load failure)", async () => {
    stub({ listDisputes: vi.fn().mockRejectedValue(new Error(SECRET)) });
    expect(await listDisputes("OPEN", "")).toEqual({ status: "unavailable" });
  });
});

describe("getDispute", () => {
  it("getDispute maps NOT_FOUND to not_found (Deep link opens one dispute)", async () => {
    stub({ getDispute: reject(Code.NotFound) });
    const res = await getDispute("nope");
    expect(res).toEqual({ status: "not_found" });
  });

  it("calls GetDispute with the id and returns the view (Deep link opens one dispute)", async () => {
    const m = stub({
      getDispute: vi.fn().mockResolvedValue({ dispute: pbDispute() }),
    });
    const res = await getDispute("d-1");
    expect(m.getDispute.mock.calls[0][0].disputeId).toBe("d-1");
    expect(m.getDispute.mock.calls[0][1]).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
    if (res.status === "ok") expect(res.dispute.id).toBe("d-1");
  });

  it.each([
    [Code.PermissionDenied, "forbidden"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.Unavailable, "unavailable"],
  ])(
    "maps code %s to %s (Gateway refusal and expired session)",
    async (code, want) => {
      stub({ getDispute: reject(code) });
      expect(await getDispute("d-1")).toEqual({ status: want });
    },
  );
});

describe("listOrderDisputes", () => {
  it("sends the order id and returns views in server order (Disputes are listed with status and ruling)", async () => {
    const m = stub({
      listOrderDisputes: vi.fn().mockResolvedValue({
        disputes: [
          pbDispute({ id: "d-9", status: DisputeStatus.INVESTIGATING }),
          pbDispute({ id: "d-8", status: DisputeStatus.REJECTED }),
        ],
      }),
    });
    const res = await listOrderDisputes("o-1");
    expect(m.listOrderDisputes.mock.calls[0][0].orderId).toBe("o-1");
    expect(m.listOrderDisputes.mock.calls[0][1]).toMatchObject({
      timeoutMs: 5000,
    });
    expect(res.status).toBe("ok");
    if (res.status !== "ok") return;
    expect(res.disputes.map((d) => d.id)).toEqual(["d-9", "d-8"]);
    expect(res.disputes[0].statusText).toBe("Đang điều tra");
    expect(res.disputes[1].statusText).toBe("Đã từ chối");
  });

  it("returns an empty list for an order with no disputes (Order has no disputes)", async () => {
    stub({ listOrderDisputes: vi.fn().mockResolvedValue({ disputes: [] }) });
    expect(await listOrderDisputes("o-1")).toEqual({
      status: "ok",
      disputes: [],
    });
  });

  it("maps SELLER_FAVOUR ruling text (Disputes are listed with status and ruling)", async () => {
    stub({
      listOrderDisputes: vi.fn().mockResolvedValue({
        disputes: [
          pbDispute({
            status: DisputeStatus.RESOLVED,
            ruling: DisputeRuling.SELLER_FAVOUR,
          }),
        ],
      }),
    });
    const res = await listOrderDisputes("o-1");
    if (res.status !== "ok") throw new Error("expected ok");
    expect(res.disputes[0].rulingText).toBe("Người bán thắng");
  });

  it.each([Code.Unavailable, Code.PermissionDenied, Code.DeadlineExceeded])(
    "listOrderDisputes maps failures to an error result (Dispute list failure leaves the order page intact) code=%s",
    async (code) => {
      stub({ listOrderDisputes: reject(code) });
      const res = await listOrderDisputes("o-1");
      expect(res).toEqual({ status: "unavailable" });
      expect(JSON.stringify(res)).not.toContain("secret-upstream");
    },
  );

  it("never rejects on a non-Connect throw (Dispute list failure leaves the order page intact)", async () => {
    stub({ listOrderDisputes: vi.fn().mockRejectedValue(new Error(SECRET)) });
    await expect(listOrderDisputes("o-1")).resolves.toEqual({
      status: "unavailable",
    });
  });
});

describe("createDispute", () => {
  it("sends order, defendant, return and reason with a 5 s timeout (Escalation creates a dispute)", async () => {
    const m = stub({
      createDispute: vi.fn().mockResolvedValue({ dispute: pbDispute() }),
    });
    const res = await createDispute({
      orderId: "o-1",
      defendantId: "s-1",
      returnId: "r-1",
      reason: "hang loi",
    });
    expect(m.createDispute).toHaveBeenCalledTimes(1);
    const [req, opts] = m.createDispute.mock.calls[0];
    expect(req).toMatchObject({
      orderId: "o-1",
      defendantId: "s-1",
      returnId: "r-1",
      reason: "hang loi",
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
    if (res.status === "ok")
      expect(res.dispute.statusText).toBe("Đang chờ xử lý");
  });

  it.each([
    [Code.FailedPrecondition, "failed_precondition"],
    [Code.AlreadyExists, "already_exists"],
    [Code.PermissionDenied, "permission_denied"],
    [Code.NotFound, "not_found"],
    [Code.InvalidArgument, "invalid_argument"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "unavailable"],
  ])(
    "maps code %s to %s without server text (Server says the return cannot be escalated)",
    async (code, want) => {
      stub({ createDispute: reject(code) });
      const res = await createDispute({
        orderId: "o-1",
        defendantId: "s-1",
        returnId: "r-1",
        reason: "x",
      });
      expect(res).toEqual({ status: want });
    },
  );
});

describe("resolveDispute", () => {
  it("sends status, resolution and ruling together for RESOLVED (Seller-favour and reject send the right payload)", async () => {
    const m = stub({
      resolveDispute: vi.fn().mockResolvedValue({
        dispute: pbDispute({
          status: DisputeStatus.RESOLVED,
          ruling: DisputeRuling.SELLER_FAVOUR,
        }),
      }),
    });
    const res = await resolveDispute({
      disputeId: "d-1",
      status: "RESOLVED",
      resolution: "seller ok",
      ruling: "SELLER_FAVOUR",
    });
    const [req, opts] = m.resolveDispute.mock.calls[0];
    expect(req).toMatchObject({
      disputeId: "d-1",
      status: DisputeStatus.RESOLVED,
      resolution: "seller ok",
      ruling: DisputeRuling.SELLER_FAVOUR,
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res.status).toBe("ok");
  });

  it("sends no ruling for REJECTED (Seller-favour and reject send the right payload)", async () => {
    const m = stub({
      resolveDispute: vi.fn().mockResolvedValue({
        dispute: pbDispute({ status: DisputeStatus.REJECTED }),
      }),
    });
    await resolveDispute({
      disputeId: "d-1",
      status: "REJECTED",
      resolution: "no",
    });
    const [req] = m.resolveDispute.mock.calls[0];
    expect(req.status).toBe(DisputeStatus.REJECTED);
    expect(req.ruling ?? DisputeRuling.UNSPECIFIED).toBe(
      DisputeRuling.UNSPECIFIED,
    );
  });

  it("sends INVESTIGATING (Start investigating)", async () => {
    const m = stub({
      resolveDispute: vi.fn().mockResolvedValue({
        dispute: pbDispute({ status: DisputeStatus.INVESTIGATING }),
      }),
    });
    await resolveDispute({
      disputeId: "d-1",
      status: "INVESTIGATING",
      resolution: "",
    });
    expect(m.resolveDispute.mock.calls[0][0].status).toBe(
      DisputeStatus.INVESTIGATING,
    );
  });

  it.each([
    [Code.FailedPrecondition, "failed_precondition"],
    [Code.PermissionDenied, "permission_denied"],
    [Code.NotFound, "not_found"],
    [Code.InvalidArgument, "invalid_argument"],
    [Code.Unauthenticated, "unauthenticated"],
    [Code.Unavailable, "unavailable"],
    [Code.DeadlineExceeded, "timeout"],
  ])(
    "maps code %s to %s without server text (Server refuses the transition)",
    async (code, want) => {
      stub({ resolveDispute: reject(code) });
      const res = await resolveDispute({
        disputeId: "d-1",
        status: "RESOLVED",
        resolution: "x",
        ruling: "BUYER_FAVOUR",
      });
      expect(res).toEqual({ status: want });
    },
  );
});
