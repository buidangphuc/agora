import { describe, expect, it } from "vitest";

import {
  KYC_REASON_CODES,
  KYC_REASON_LABELS,
  isKycReasonCode,
} from "./kycReasons";

// Contract (implementer creates src/features/admin/kycReasons.ts):
//   KYC_REASON_CODES: readonly string[] (fixed order)
//   KYC_REASON_LABELS: Record<code, Vietnamese label>
//   isKycReasonCode(v: unknown): v is code

describe("kyc reason codes", () => {
  it("lists the fixed reason codes in order (KYC queue: Reject with a reason code)", () => {
    expect([...KYC_REASON_CODES]).toEqual([
      "document_unreadable",
      "document_expired",
      "document_mismatch",
      "document_type_invalid",
      "suspected_fraud",
      "other",
    ]);
  });

  it("labels every code in Vietnamese (KYC queue: Reject with a reason code)", () => {
    expect(KYC_REASON_LABELS.document_unreadable).toBe(
      "Giấy tờ không đọc được",
    );
    for (const c of KYC_REASON_CODES) {
      expect(KYC_REASON_LABELS[c].trim().length).toBeGreaterThan(0);
    }
    expect(Object.keys(KYC_REASON_LABELS).sort()).toEqual(
      [...KYC_REASON_CODES].sort(),
    );
  });

  it("accepts only members of the list (KYC queue: Forged decision or reason is refused by the action)", () => {
    for (const c of KYC_REASON_CODES) expect(isKycReasonCode(c)).toBe(true);
    for (const bad of [
      "Bad Reason!",
      "",
      " other",
      "OTHER",
      "ban",
      undefined,
      null,
      42,
    ]) {
      expect(isKycReasonCode(bad)).toBe(false);
    }
  });
});
