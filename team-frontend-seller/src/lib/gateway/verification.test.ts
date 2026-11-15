import { Code, ConnectError, createPromiseClient } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import {
  createKycUploadUrl,
  getVerificationStatus,
  submitKyc,
} from "./verification.js";

vi.mock("@connectrpc/connect", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@connectrpc/connect")>()),
  createPromiseClient: vi.fn(),
}));
vi.mock("@connectrpc/connect-node", () => ({
  createConnectTransport: vi.fn(() => ({})),
}));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  getPrincipal: vi.fn(() => ({ id: "u1", name: "n", scopes: [] })),
  SESSION_COOKIE: "session",
}));

function stubClient(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  vi.mocked(createPromiseClient).mockReturnValue(rpcs as never);
  return rpcs;
}

beforeEach(() => vi.clearAllMocks());

describe("seller verification gateway module", () => {
  it("A seller who never submitted sees NOT_SUBMITTED and the form: maps the status response and asks for the given user id only", async () => {
    const rpc = stubClient({
      getVerificationStatus: vi.fn().mockResolvedValue({
        status: VerificationStatus.NOT_SUBMITTED,
        badge: false,
        rejectReasonCode: "",
      }),
    });
    const view = await getVerificationStatus("u1");
    expect(view).toMatchObject({ status: VerificationStatus.NOT_SUBMITTED });
    expect(rpc.getVerificationStatus).toHaveBeenCalledTimes(1);
    expect(rpc.getVerificationStatus).toHaveBeenCalledWith({ userId: "u1" });
  });

  it("A rejected submission shows the reason and allows resubmission: exposes the reject reason code", async () => {
    stubClient({
      getVerificationStatus: vi.fn().mockResolvedValue({
        status: VerificationStatus.REJECTED,
        badge: false,
        rejectReasonCode: "document_unreadable",
      }),
    });
    const view = await getVerificationStatus("u1");
    expect(view).toMatchObject({
      status: VerificationStatus.REJECTED,
      rejectReasonCode: "document_unreadable",
    });
  });

  it("Status read failure is shown as an error, not as not-submitted: rejects instead of returning a status", async () => {
    stubClient({
      getVerificationStatus: vi
        .fn()
        .mockRejectedValue(new ConnectError("down", Code.Unavailable)),
    });
    await expect(getVerificationStatus("u1")).rejects.toBeInstanceOf(
      ConnectError,
    );
  });

  it("Three valid files are uploaded and submitted: createKycUploadUrl forwards content type and size and returns uploadUrl and docRef", async () => {
    const rpc = stubClient({
      createKycUploadUrl: vi
        .fn()
        .mockResolvedValue({ uploadUrl: "https://s3/x", docRef: "ref-1" }),
    });
    const out = await createKycUploadUrl("image/png", 1234);
    expect(out).toEqual({ uploadUrl: "https://s3/x", docRef: "ref-1" });
    expect(rpc.createKycUploadUrl).toHaveBeenCalledWith({
      contentType: "image/png",
      sizeBytes: 1234n,
    });
  });

  it("Three valid files are uploaded and submitted: submitKyc sends the first ref as docRef and the rest as additionalDocRefs", async () => {
    const rpc = stubClient({
      submitKyc: vi
        .fn()
        .mockResolvedValue({ id: "s1", status: VerificationStatus.PENDING }),
    });
    await submitKyc("national_id", "r1", ["r2", "r3"]);
    expect(rpc.submitKyc).toHaveBeenCalledTimes(1);
    expect(rpc.submitKyc).toHaveBeenCalledWith({
      docType: "national_id",
      docRef: "r1",
      additionalDocRefs: ["r2", "r3"],
    });
  });

  it("The server refuses a doc_ref: submitKyc propagates the gateway error code", async () => {
    stubClient({
      submitKyc: vi
        .fn()
        .mockRejectedValue(new ConnectError("no", Code.InvalidArgument)),
    });
    await expect(submitKyc("national_id", "r1", [])).rejects.toMatchObject({
      code: Code.InvalidArgument,
    });
  });
});
