import {
  Result as PlatformResult,
  type ResultProps as PlatformResultProps,
  type ResultStatus,
} from "@fe-platform/ui";

export type { ResultStatus };

export type ResultProps = Omit<PlatformResultProps, "statusLabel">;

const statusLabels: Record<ResultStatus, string> = {
  success: "Thành công",
  error: "Lỗi",
  warning: "Cảnh báo",
  info: "Thông tin",
  "403": "Không có quyền truy cập",
  "404": "Không tìm thấy",
};

/**
 * Ant Design-style Result pattern.
 * Used for post-action status: Order Placed, Payment Failed, KYC Submission, 404.
 */
export function Result(props: ResultProps) {
  return <PlatformResult {...props} statusLabel={statusLabels[props.status]} />;
}
