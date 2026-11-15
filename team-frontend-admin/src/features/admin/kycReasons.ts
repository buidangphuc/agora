export const KYC_REASON_CODES = [
  "document_unreadable",
  "document_expired",
  "document_mismatch",
  "document_type_invalid",
  "suspected_fraud",
  "other",
] as const;

export type KycReasonCode = (typeof KYC_REASON_CODES)[number];

export const KYC_REASON_LABELS: Record<KycReasonCode, string> = {
  document_unreadable: "Giấy tờ không đọc được",
  document_expired: "Giấy tờ đã hết hạn",
  document_mismatch: "Giấy tờ không khớp thông tin",
  document_type_invalid: "Loại giấy tờ không hợp lệ",
  suspected_fraud: "Nghi ngờ gian lận",
  other: "Lý do khác",
};

export function isKycReasonCode(v: unknown): v is KycReasonCode {
  return (
    typeof v === "string" && (KYC_REASON_CODES as readonly string[]).includes(v)
  );
}
