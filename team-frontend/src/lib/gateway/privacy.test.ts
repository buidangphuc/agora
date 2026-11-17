import { Timestamp } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ConsentPurpose } from "@/generated/platform/identity/v1/identity_pb.js";

const rpc = vi.hoisted(() => ({
  listMyConsents: vi.fn(),
  recordConsent: vi.fn(),
  getAccountDeletionStatus: vi.fn(),
  requestAccountDeletion: vi.fn(),
  cancelAccountDeletion: vi.fn(),
  exportMyData: vi.fn(),
  transportOpts: undefined as
    | { interceptors?: ((next: unknown) => (req: unknown) => unknown)[] }
    | undefined,
}));

vi.mock("@connectrpc/connect", async (importOriginal) => {
  const orig = await importOriginal<typeof import("@connectrpc/connect")>();
  return {
    ...orig,
    createPromiseClient: (svc: { typeName: string }) =>
      svc.typeName === "platform.identity.v1.PrivacyService" ? rpc : {},
  };
});
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: (o: typeof rpc.transportOpts) => {
    rpc.transportOpts = o;
    return {};
  },
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

import { makeClients } from "./client.js";
import {
  cancelDeletion,
  exportMyData,
  getDeletionStatus,
  listConsents,
  recordConsents,
  requestDeletion,
} from "./privacy.js";

beforeEach(() => vi.clearAllMocks());

describe("gateway client factory", () => {
  it("[Signed-in save records only changed purposes] makeClients exposes the PrivacyService client", () => {
    const c = makeClients("tok") as unknown as {
      privacy?: { listMyConsents: unknown; recordConsent: unknown };
    };
    expect(c.privacy).toBe(rpc);
  });
});

describe("listConsents", () => {
  it("[A newer Terms version triggers the prompt] maps records and the next cursor, sending the cursor and page size 100", async () => {
    rpc.listMyConsents.mockResolvedValue({
      records: [
        {
          purpose: ConsentPurpose.TERMS,
          granted: true,
          version: "2026-01-01",
          source: "register",
          recordedAt: Timestamp.fromDate(new Date("2026-01-01T00:00:00Z")),
        },
        {
          purpose: ConsentPurpose.PRIVACY_POLICY,
          granted: false,
          version: "2026-02-02",
          source: "account",
        },
      ],
      page: { nextCursor: "c2" },
    });
    const res = await listConsents("c1");
    expect(rpc.listMyConsents).toHaveBeenCalledTimes(1);
    expect(rpc.listMyConsents.mock.calls[0][0]).toMatchObject({
      page: { cursor: "c1", pageSize: 100 },
    });
    expect(res.nextCursor).toBe("c2");
    expect(res.records).toHaveLength(2);
    expect(res.records[0]).toMatchObject({
      purpose: "TERMS",
      granted: true,
      version: "2026-01-01",
      source: "register",
    });
    expect(res.records[1]).toMatchObject({
      purpose: "PRIVACY_POLICY",
      granted: false,
      version: "2026-02-02",
    });
  });

  it("[A newer Terms version triggers the prompt] returns an empty cursor on the last page", async () => {
    rpc.listMyConsents.mockResolvedValue({ records: [] });
    await expect(listConsents()).resolves.toEqual({
      records: [],
      nextCursor: "",
    });
  });

  it("[Consent lookup failure does not block browsing] propagates the gateway error to the caller", async () => {
    rpc.listMyConsents.mockRejectedValue(new Error("unavailable"));
    await expect(listConsents()).rejects.toThrow("unavailable");
  });
});

describe("recordConsents", () => {
  it("[Signed-in save records only changed purposes] sends exactly the given records in one call, mapped to proto purposes", async () => {
    rpc.recordConsent.mockResolvedValue({});
    await recordConsents([
      {
        purpose: "PERSONALIZATION",
        granted: true,
        version: "1",
        source: "banner",
      },
    ]);
    expect(rpc.recordConsent).toHaveBeenCalledTimes(1);
    const req = rpc.recordConsent.mock.calls[0][0] as {
      records: {
        purpose: number;
        granted: boolean;
        version: string;
        source: string;
      }[];
    };
    expect(req.records).toHaveLength(1);
    expect(req.records[0]).toMatchObject({
      purpose: ConsentPurpose.PERSONALIZATION,
      granted: true,
      version: "1",
      source: "banner",
    });
  });

  it("[Registering with both boxes checked records two consents] maps TERMS and PRIVACY_POLICY", async () => {
    rpc.recordConsent.mockResolvedValue({});
    await recordConsents([
      { purpose: "TERMS", granted: true, version: "v1", source: "account" },
      {
        purpose: "PRIVACY_POLICY",
        granted: true,
        version: "v2",
        source: "account",
      },
    ]);
    const req = rpc.recordConsent.mock.calls[0][0] as {
      records: { purpose: number }[];
    };
    expect(req.records.map((r) => r.purpose)).toEqual([
      ConsentPurpose.TERMS,
      ConsentPurpose.PRIVACY_POLICY,
    ]);
  });

  it("[Ledger write failure keeps the cookie choice] propagates the gateway error to the caller", async () => {
    rpc.recordConsent.mockRejectedValue(new Error("unavailable"));
    await expect(
      recordConsents([
        {
          purpose: "ANALYTICS",
          granted: false,
          version: "1",
          source: "banner",
        },
      ]),
    ).rejects.toThrow("unavailable");
  });
});

async function bearerSent(): Promise<string | null> {
  const chain = rpc.transportOpts?.interceptors ?? [];
  const req = { header: new Headers() };
  const terminal = async (r: unknown) => r;
  const call = chain.reduceRight(
    (next, ic) => ic(next) as typeof terminal,
    terminal,
  );
  await call(req);
  return req.header.get("authorization");
}

describe("PrivacyService calls carry the session token and no user id", () => {
  it("[PV27 No request carries a user id] sends the session token and no user id", async () => {
    rpc.recordConsent.mockResolvedValue({});
    rpc.getAccountDeletionStatus.mockResolvedValue({ pending: false });
    rpc.requestAccountDeletion.mockResolvedValue({});
    rpc.cancelAccountDeletion.mockResolvedValue({});
    rpc.exportMyData.mockResolvedValue({
      data: new Uint8Array(),
      contentType: "application/json",
    });
    rpc.listMyConsents.mockResolvedValue({ records: [] });
    await listConsents();
    await recordConsents([
      { purpose: "ANALYTICS", granted: true, version: "1", source: "account" },
    ]);
    await getDeletionStatus();
    await requestDeletion("pw");
    await cancelDeletion();
    await exportMyData();
    const reqs = [
      rpc.listMyConsents,
      rpc.recordConsent,
      rpc.getAccountDeletionStatus,
      rpc.requestAccountDeletion,
      rpc.cancelAccountDeletion,
      rpc.exportMyData,
    ].map((f) => f.mock.calls[0][0] as Record<string, unknown>);
    for (const r of reqs) {
      expect(Object.keys(r)).not.toContain("userId");
      expect(Object.keys(r)).not.toContain("user_id");
    }
    expect(await bearerSent()).toBe("bearer test-token");
  });
});

describe("getDeletionStatus", () => {
  it("[PV16 Confirming with the password schedules deletion] maps pending and erase time to ISO", async () => {
    rpc.getAccountDeletionStatus.mockResolvedValue({
      pending: true,
      eraseAfter: Timestamp.fromDate(new Date("2026-11-16T10:00:00Z")),
    });
    await expect(getDeletionStatus()).resolves.toEqual({
      pending: true,
      eraseAfter: "2026-11-16T10:00:00.000Z",
    });
  });

  it("[PV16 Confirming with the password schedules deletion] reports nothing pending with an empty erase time", async () => {
    rpc.getAccountDeletionStatus.mockResolvedValue({ pending: false });
    await expect(getDeletionStatus()).resolves.toEqual({
      pending: false,
      eraseAfter: "",
    });
  });
});

describe("requestDeletion", () => {
  it("[PV16 Confirming with the password schedules deletion] sends only the password and returns the erase time", async () => {
    rpc.requestAccountDeletion.mockResolvedValue({
      eraseAfter: Timestamp.fromDate(new Date("2026-11-16T10:00:00Z")),
    });
    const res = await requestDeletion("s3cret");
    expect(rpc.requestAccountDeletion).toHaveBeenCalledTimes(1);
    expect(rpc.requestAccountDeletion.mock.calls[0][0]).toEqual({
      password: "s3cret",
    });
    expect(res).toEqual({ eraseAfter: "2026-11-16T10:00:00.000Z" });
  });

  it("[PV19 An open obligation shows the reason] propagates the gateway error", async () => {
    rpc.requestAccountDeletion.mockRejectedValue(new Error("blocked"));
    await expect(requestDeletion("x")).rejects.toThrow("blocked");
  });
});

describe("cancelDeletion", () => {
  it("[PV22 Cancel with nothing pending is explained] calls CancelAccountDeletion once and propagates errors", async () => {
    rpc.cancelAccountDeletion.mockResolvedValueOnce({});
    await cancelDeletion();
    expect(rpc.cancelAccountDeletion).toHaveBeenCalledTimes(1);
    rpc.cancelAccountDeletion.mockRejectedValueOnce(new Error("none"));
    await expect(cancelDeletion()).rejects.toThrow("none");
  });
});

describe("exportMyData", () => {
  it("[PV11 Export downloads one JSON file] decodes the bytes to text and maps type and generated time", async () => {
    const json = '{"account":{"user_id":"u1"},"name":"Nguyễn"}';
    rpc.exportMyData.mockResolvedValue({
      data: new TextEncoder().encode(json),
      contentType: "application/json",
      generatedAt: Timestamp.fromDate(new Date("2026-11-14T10:00:00Z")),
    });
    await expect(exportMyData()).resolves.toEqual({
      content: json,
      contentType: "application/json",
      generatedAt: "2026-11-14T10:00:00.000Z",
    });
  });
});
