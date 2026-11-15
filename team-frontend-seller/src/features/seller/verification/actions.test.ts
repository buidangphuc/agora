import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";
import { createKycUploadUrl, submitKyc } from "@/lib/gateway/verification";

import { createKycUploadUrlAction, submitKycAction } from "./actions";

vi.mock("next/cache", () => ({ revalidatePath: vi.fn() }));
vi.mock("@/lib/gateway/verification", () => ({
  createKycUploadUrl: vi.fn(),
  submitKyc: vi.fn(),
  getVerificationStatus: vi.fn(),
}));

beforeEach(() => vi.clearAllMocks());

describe("submitKycAction", () => {
  it("Three valid files are uploaded and submitted: first ref is doc_ref and the rest additional", async () => {
    vi.mocked(submitKyc).mockResolvedValue({
      id: "s1",
      status: VerificationStatus.PENDING,
    } as never);
    const res = await submitKycAction("national_id", ["r1", "r2", "r3"]);
    expect(res.ok).toBe(true);
    expect(submitKyc).toHaveBeenCalledTimes(1);
    expect(submitKyc).toHaveBeenCalledWith("national_id", "r1", ["r2", "r3"]);
  });

  it("maps INVALID_ARGUMENT and PERMISSION_DENIED from SubmitKyc to the invalid documents message", async () => {
    for (const code of [Code.InvalidArgument, Code.PermissionDenied]) {
      vi.mocked(submitKyc).mockRejectedValueOnce(new ConnectError("raw", code));
      const res = await submitKycAction("national_id", ["r1"]);
      expect(res).toEqual({
        ok: false,
        error: "Hồ sơ không hợp lệ, vui lòng tải lại tệp",
      });
    }
  });

  it("Consent unticked blocks submission: an empty ref list is refused without calling the gateway", async () => {
    const res = await submitKycAction("national_id", []);
    expect(res.ok).toBe(false);
    expect(submitKyc).not.toHaveBeenCalled();
  });

  it("Image bytes and keys never reach logs or the server action payload: failures log no doc_ref", async () => {
    const spies = (["log", "error", "warn", "info"] as const).map((m) =>
      vi.spyOn(console, m).mockImplementation(() => {}),
    );
    vi.mocked(submitKyc).mockRejectedValue(
      new ConnectError("boom", Code.InvalidArgument),
    );
    await submitKycAction("national_id", ["secret-ref-1"]);
    for (const s of spies) {
      expect(JSON.stringify(s.mock.calls)).not.toContain("secret-ref-1");
      s.mockRestore();
    }
  });
});

describe("createKycUploadUrlAction", () => {
  it("Three valid files are uploaded and submitted: returns uploadUrl and docRef for a content type and size", async () => {
    vi.mocked(createKycUploadUrl).mockResolvedValue({
      uploadUrl: "https://s3/x",
      docRef: "r1",
    });
    const res = await createKycUploadUrlAction("image/png", 10);
    expect(res).toEqual({
      ok: true,
      data: { uploadUrl: "https://s3/x", docRef: "r1" },
    });
    expect(createKycUploadUrl).toHaveBeenCalledWith("image/png", 10);
  });

  it("A wrong file type or an empty file is rejected: invalid input never reaches the gateway", async () => {
    for (const [type, size] of [
      ["image/gif", 10],
      ["image/png", 0],
      ["image/png", 5 * 1024 * 1024 + 1],
    ] as const) {
      const res = await createKycUploadUrlAction(type, size);
      expect(res.ok).toBe(false);
    }
    expect(createKycUploadUrl).not.toHaveBeenCalled();
  });

  it("A failed PUT stops the submission: a gateway failure becomes a Vietnamese error, not a throw", async () => {
    vi.mocked(createKycUploadUrl).mockRejectedValue(
      new ConnectError("raw", Code.Unavailable),
    );
    const res = await createKycUploadUrlAction("image/png", 10);
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).not.toContain("raw");
  });
});
