import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  DisputeRuling,
  DisputeStatus,
} from "@/generated/platform/engagement/v1/engagement_pb.js";

import { makeClients } from "./client.js";
import { createDispute, listOrderDisputes } from "./disputes.js";

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
