import { VerificationStatus } from "@/generated/platform/verification/v1/verification_pb.js";

export const MAX_KYC_FILES = 3;
export const MAX_KYC_FILE_BYTES = 5 * 1024 * 1024;
export const KYC_ACCEPTED_TYPES = [
  "image/jpeg",
  "image/png",
  "application/pdf",
];

const GENERIC_REASON = "Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại";

const REASON_TEXTS: Record<string, string> = {
  document_unreadable:
    "Tài liệu mờ hoặc không đọc được, vui lòng chụp lại rõ nét",
  document_expired: "Giấy tờ đã hết hạn, vui lòng gửi giấy tờ còn hiệu lực",
  document_mismatch: "Thông tin trên giấy tờ không khớp với hồ sơ",
};

export function rejectReasonText(code: string): string {
  return REASON_TEXTS[code] ?? GENERIC_REASON;
}

export function verificationStatusLabel(status: VerificationStatus): string {
  switch (status) {
    case VerificationStatus.NOT_SUBMITTED:
      return "Chưa gửi hồ sơ";
    case VerificationStatus.PENDING:
      return "Đang chờ duyệt";
    case VerificationStatus.VERIFIED:
      return "Đã xác minh";
    case VerificationStatus.REJECTED:
      return "Bị từ chối";
    default:
      return "Chưa gửi hồ sơ";
  }
}

export interface KycFileLike {
  name: string;
  type: string;
  size: number;
}

export interface KycValidation {
  ok: boolean;
  fileErrors: (string | null)[];
  limitError: string | null;
}

export function validateKycFile(file: KycFileLike): string | null {
  if (!KYC_ACCEPTED_TYPES.includes(file.type))
    return "Chỉ nhận JPG, PNG hoặc PDF";
  if (file.size <= 0) return "Tệp rỗng";
  if (file.size > MAX_KYC_FILE_BYTES) return "Tệp tối đa 5 MiB";
  return null;
}

export function validateKycFiles(files: KycFileLike[]): KycValidation {
  if (files.length > MAX_KYC_FILES) {
    return {
      ok: false,
      fileErrors: files.map(() => null),
      limitError: `Tối đa ${MAX_KYC_FILES} tệp`,
    };
  }
  const fileErrors = files.map(validateKycFile);
  return {
    ok: files.length > 0 && fileErrors.every((e) => e === null),
    fileErrors,
    limitError: null,
  };
}
