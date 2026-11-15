import { Timestamp } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import { makeClients } from "./client.js";
import { getKycDocumentUrl, listKycSubmissions, reviewKyc } from "./kyc.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

// Contract under test (implementer creates src/lib/gateway/kyc.ts), using makeClients().verification:
//   listKycSubmissions(status: "PENDING" | "VERIFIED" | "REJECTED", cursor) =>
//     { status: "ok"; items: KycSubmissionView[]; nextCursor: string }
//   | { status: "unauthenticated" | "forbidden" | "unavailable" }
//   KycSubmissionView = { id, userId, docType: string; status: "PENDING" | "VERIFIED" | "REJECTED";
//     documentCount: number; reasonCode: string; createdAt: string; reviewedAt: string } (ISO-8601 UTC, "" when unset)
//   getKycDocumentUrl(id, index) => { status: "ok"; url: string; expiresAt: string /* ISO */ } | { status: <failure> }
//   reviewKyc(id, decision: "approve" | "reject", reasonCode) => { status: "ok" } | { status: <failure> }
//   <failure> = unauthenticated | forbidden | not_found | failed_precondition | invalid | unavailable
// 5 s timeout, never throws, results never carry err.message, nothing is logged.

function stub(verification: Record<string, ReturnType<typeof vi.fn>>) {
  vi.mocked(makeClients).mockReturnValue({ verification } as never);
  return verification;
}

const FAILURES: [Code, string][] = [
  [Code.Unauthenticated, "unauthenticated"],
  [Code.PermissionDenied, "forbidden"],
  [Code.NotFound, "not_found"],
  [Code.FailedPrecondition, "failed_precondition"],
  [Code.InvalidArgument, "invalid"],
  [Code.Unavailable, "unavailable"],
  [Code.DeadlineExceeded, "unavailable"],
  [Code.Internal, "unavailable"],
];

let spies: ReturnType<typeof vi.spyOn>[] = [];
beforeEach(() => {
  vi.clearAllMocks();
  spies = (["log", "info", "warn", "error", "debug"] as const).map((m) =>
    vi.spyOn(console, m).mockImplementation(() => {}),
  );
});
afterEach(() => {
  for (const s of spies) s.mockRestore();
});

function logged() {
  return JSON.stringify(spies.flatMap((s) => s.mock.calls));
}

describe("listKycSubmissions gateway wrapper", () => {
  it("KYC queue: Unknown status or oversized cursor falls back - sends status, cursor, page size 20 and 5 s timeout, keeps order", async () => {
    const v = stub({
      listKycSubmissions: vi.fn().mockResolvedValue({
        submissions: [
          {
            id: "k1",
            userId: "u1",
            docType: "cccd",
            status: VerificationStatus.PENDING,
            documentCount: 2,
            reasonCode: "",
            createdAt: Timestamp.fromDate(new Date("2026-10-01T00:00:00Z")),
          },
          {
            id: "k2",
            userId: "u2",
            docType: "passport",
            status: VerificationStatus.REJECTED,
            documentCount: 1,
            reasonCode: "document_expired",
            createdAt: Timestamp.fromDate(new Date("2026-10-02T00:00:00Z")),
            reviewedAt: Timestamp.fromDate(new Date("2026-10-03T00:00:00Z")),
          },
        ],
        page: { nextCursor: "20" },
      }),
    });
    const res = await listKycSubmissions("PENDING", "c0");
    const [req, opts] = v.listKycSubmissions.mock.calls[0];
    expect(req.status).toBe(VerificationStatus.PENDING);
    expect(req.page).toMatchObject({ cursor: "c0", pageSize: 20 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res).toEqual({
      status: "ok",
      nextCursor: "20",
      items: [
        {
          id: "k1",
          userId: "u1",
          docType: "cccd",
          status: "PENDING",
          documentCount: 2,
          reasonCode: "",
          createdAt: "2026-10-01T00:00:00.000Z",
          reviewedAt: "",
        },
        {
          id: "k2",
          userId: "u2",
          docType: "passport",
          status: "REJECTED",
          documentCount: 1,
          reasonCode: "document_expired",
          createdAt: "2026-10-02T00:00:00.000Z",
          reviewedAt: "2026-10-03T00:00:00.000Z",
        },
      ],
    });
  });

  it("KYC queue: Unknown status or oversized cursor falls back - maps VERIFIED and REJECTED filters to the enum", async () => {
    const v = stub({
      listKycSubmissions: vi.fn().mockResolvedValue({ submissions: [] }),
    });
    const res = await listKycSubmissions("VERIFIED", "");
    await listKycSubmissions("REJECTED", "");
    expect(v.listKycSubmissions.mock.calls[0][0].status).toBe(
      VerificationStatus.VERIFIED,
    );
    expect(v.listKycSubmissions.mock.calls[1][0].status).toBe(
      VerificationStatus.REJECTED,
    );
    expect(res).toEqual({ status: "ok", items: [], nextCursor: "" });
  });

  it("KYC queue: Non-admin sees the 403 result - PERMISSION_DENIED is forbidden", async () => {
    stub({
      listKycSubmissions: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.PermissionDenied)),
    });
    expect(await listKycSubmissions("PENDING", "")).toEqual({
      status: "forbidden",
    });
  });

  it("KYC queue: Gateway refusal is shown as the same 403 - the gateway scope refusal maps to forbidden", async () => {
    stub({
      listKycSubmissions: vi
        .fn()
        .mockRejectedValue(
          new ConnectError("scope admin required", Code.PermissionDenied),
        ),
    });
    const res = await listKycSubmissions("PENDING", "");
    expect(res).toEqual({ status: "forbidden" });
    expect(JSON.stringify(res)).not.toContain("scope");
  });

  it("KYC queue: Expired session is sent to login - UNAUTHENTICATED is unauthenticated", async () => {
    stub({
      listKycSubmissions: vi
        .fn()
        .mockRejectedValue(new ConnectError("expired", Code.Unauthenticated)),
    });
    expect(await listKycSubmissions("PENDING", "")).toEqual({
      status: "unauthenticated",
    });
  });

  it("KYC queue: List failure shows a retry message - unavailable, deadline and plain errors never leak upstream text", async () => {
    for (const e of [
      new ConnectError("dial tcp 10.0.0.9:9000", Code.Unavailable),
      new ConnectError("deadline", Code.DeadlineExceeded),
      new Error("boom at /srv/app.js:12"),
    ]) {
      stub({ listKycSubmissions: vi.fn().mockRejectedValue(e) });
      const res = await listKycSubmissions("PENDING", "");
      expect(res).toEqual({ status: "unavailable" });
    }
  });

  it("KYC queue: List failure shows a retry message - never throws when the client itself is broken", async () => {
    vi.mocked(makeClients).mockImplementation(() => {
      throw new Error("no transport");
    });
    expect(await listKycSubmissions("PENDING", "")).toEqual({
      status: "unavailable",
    });
  });
});

describe("getKycDocumentUrl gateway wrapper", () => {
  it("KYC queue: Unknown submission or index is refused - sends id and index with 5 s timeout and returns url and expiry", async () => {
    const v = stub({
      getKycDocumentUrl: vi.fn().mockResolvedValue({
        url: "https://store.example/doc?sig=abc",
        expiresAt: Timestamp.fromDate(new Date("2026-10-01T00:05:00Z")),
      }),
    });
    const res = await getKycDocumentUrl("k1", 1);
    const [req, opts] = v.getKycDocumentUrl.mock.calls[0];
    expect(req).toMatchObject({ submissionId: "k1", index: 1 });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res).toEqual({
      status: "ok",
      url: "https://store.example/doc?sig=abc",
      expiresAt: "2026-10-01T00:05:00.000Z",
    });
    expect(v.getKycDocumentUrl).toHaveBeenCalledTimes(1);
  });

  it.each(FAILURES)(
    "KYC queue: Unknown submission or index is refused - code %s maps to the status name (also Audit store down fails closed for UNAVAILABLE)",
    async (code, name) => {
      stub({
        getKycDocumentUrl: vi
          .fn()
          .mockRejectedValue(new ConnectError("audit store down", code)),
      });
      const res = await getKycDocumentUrl("k1", 0);
      expect(res).toEqual({ status: name });
    },
  );

  it("KYC queue: Audit store down fails closed - UNAVAILABLE yields no url and exactly one call (no automatic retry)", async () => {
    const v = stub({
      getKycDocumentUrl: vi
        .fn()
        .mockRejectedValue(new ConnectError("audit", Code.Unavailable)),
    });
    const res = await getKycDocumentUrl("k1", 0);
    expect(res).toEqual({ status: "unavailable" });
    expect(res).not.toHaveProperty("url");
    expect(v.getKycDocumentUrl).toHaveBeenCalledTimes(1);
  });

  it("KYC queue: Unknown submission or index is refused - non-Connect errors are unavailable", async () => {
    stub({ getKycDocumentUrl: vi.fn().mockRejectedValue(new Error("x")) });
    expect(await getKycDocumentUrl("k1", 0)).toEqual({ status: "unavailable" });
  });

  it("KYC queue: Unknown submission or index is refused - logs neither the url nor the id", async () => {
    stub({
      getKycDocumentUrl: vi.fn().mockResolvedValue({
        url: "https://store.example/doc?sig=SECRET",
        expiresAt: Timestamp.fromDate(new Date("2026-10-01T00:05:00Z")),
      }),
    });
    await getKycDocumentUrl("sub-123", 0);
    stub({
      getKycDocumentUrl: vi
        .fn()
        .mockRejectedValue(new ConnectError("sub-123 gone", Code.NotFound)),
    });
    await getKycDocumentUrl("sub-123", 0);
    expect(logged()).not.toContain("SECRET");
    expect(logged()).not.toContain("sub-123");
  });
});

describe("reviewKyc gateway wrapper", () => {
  it("KYC queue: Reject with a reason code - sends id, decision and reason_code with 5 s timeout", async () => {
    const v = stub({ reviewKyc: vi.fn().mockResolvedValue({}) });
    const res = await reviewKyc("k1", "reject", "document_unreadable");
    const [req, opts] = v.reviewKyc.mock.calls[0];
    expect(req).toMatchObject({
      id: "k1",
      decision: "reject",
      reasonCode: "document_unreadable",
    });
    expect(opts).toMatchObject({ timeoutMs: 5000 });
    expect(res).toEqual({ status: "ok" });
  });

  it("KYC queue: Reject with a reason code - approve sends an empty reason_code", async () => {
    const v = stub({ reviewKyc: vi.fn().mockResolvedValue({}) });
    await reviewKyc("k1", "approve", "");
    expect(v.reviewKyc.mock.calls[0][0]).toMatchObject({
      id: "k1",
      decision: "approve",
      reasonCode: "",
    });
  });

  it.each(FAILURES)(
    "KYC queue: Gateway refusal is shown as the same 403 - review code %s maps to the status name",
    async (code, name) => {
      stub({
        reviewKyc: vi.fn().mockRejectedValue(new ConnectError("m", code)),
      });
      expect(await reviewKyc("k1", "approve", "")).toEqual({ status: name });
    },
  );
});
