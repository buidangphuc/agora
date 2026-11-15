import { describe, expect, it } from "vitest";

import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

import {
  MAX_KYC_FILES,
  MAX_KYC_FILE_BYTES,
  rejectReasonText,
  validateKycFiles,
  verificationStatusLabel,
} from "./status";

const f = (type: string, size: number, name = "a") => ({ name, type, size });
const MIB5 = 5 * 1024 * 1024;

describe("validateKycFiles", () => {
  it("A file of exactly 5 MiB is accepted: 5242880 bytes png is valid", () => {
    expect(MAX_KYC_FILE_BYTES).toBe(5242880);
    const r = validateKycFiles([f("image/png", MIB5)]);
    expect(r.ok).toBe(true);
    expect(r.fileErrors).toEqual([null]);
    expect(r.limitError).toBeNull();
  });

  it("A file over 5 MiB is rejected before upload: 5 MiB + 1 byte gives the size message", () => {
    const r = validateKycFiles([f("image/png", MIB5 + 1)]);
    expect(r.ok).toBe(false);
    expect(r.fileErrors).toEqual(["Tệp tối đa 5 MiB"]);
  });

  it("A wrong file type or an empty file is rejected: gif gives the type message", () => {
    const r = validateKycFiles([f("image/gif", 100, "x.gif")]);
    expect(r.ok).toBe(false);
    expect(r.fileErrors).toEqual(["Chỉ nhận JPG, PNG hoặc PDF"]);
  });

  it("A wrong file type or an empty file is rejected: 0 byte gives the empty message", () => {
    const r = validateKycFiles([f("application/pdf", 0)]);
    expect(r.ok).toBe(false);
    expect(r.fileErrors).toEqual(["Tệp rỗng"]);
  });

  it("A wrong file type or an empty file is rejected: 1 byte jpeg, png and pdf are valid", () => {
    const r = validateKycFiles([
      f("image/jpeg", 1),
      f("image/png", 1),
      f("application/pdf", 1),
    ]);
    expect(r.ok).toBe(true);
  });

  it("More than three files are refused: 4 files keep only the limit message", () => {
    expect(MAX_KYC_FILES).toBe(3);
    const r = validateKycFiles([
      f("image/png", 1),
      f("image/png", 1),
      f("image/png", 1),
      f("image/gif", 1),
    ]);
    expect(r.ok).toBe(false);
    expect(r.limitError).toBe("Tối đa 3 tệp");
    expect(r.fileErrors.every((e) => e === null)).toBe(true);
  });

  it("Consent unticked blocks submission: no files is not submittable", () => {
    expect(validateKycFiles([]).ok).toBe(false);
  });
});

describe("status and reason texts", () => {
  it("A seller who never submitted sees NOT_SUBMITTED and the form: labels each status in Vietnamese", () => {
    expect(verificationStatusLabel(VerificationStatus.NOT_SUBMITTED)).toBe(
      "Chưa gửi hồ sơ",
    );
    expect(verificationStatusLabel(VerificationStatus.PENDING)).toBe(
      "Đang chờ duyệt",
    );
    expect(verificationStatusLabel(VerificationStatus.VERIFIED)).toBe(
      "Đã xác minh",
    );
    expect(verificationStatusLabel(VerificationStatus.REJECTED)).toBe(
      "Bị từ chối",
    );
  });

  it("A rejected submission shows the reason and allows resubmission: known code has its own text", () => {
    const text = rejectReasonText("document_unreadable");
    expect(text).not.toBe("");
    expect(text).not.toBe("Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại");
    expect(text).not.toContain("document_unreadable");
  });

  it("falls back to the generic text for an unknown reject reason code", () => {
    expect(rejectReasonText("zzz_unknown_code")).toBe(
      "Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại",
    );
    expect(rejectReasonText("")).toBe(
      "Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại",
    );
  });
});
