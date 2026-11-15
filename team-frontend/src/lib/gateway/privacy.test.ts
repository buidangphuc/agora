import { Timestamp } from "@bufbuild/protobuf";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ConsentPurpose } from "@/generated/platform/identity/v1/identity_pb.js";

const rpc = vi.hoisted(() => ({
  listMyConsents: vi.fn(),
  recordConsent: vi.fn(),
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
  createConnectTransport: () => ({}),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

import { makeClients } from "./client.js";
import { listConsents, recordConsents } from "./privacy.js";

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
