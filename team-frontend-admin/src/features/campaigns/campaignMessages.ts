export type CampaignErrorStatus =
  | "duplicate"
  | "invalid"
  | "not_found"
  | "forbidden"
  | "listing_unavailable"
  | "audit_unavailable"
  | "promotion_unavailable"
  | "timeout"
  | "unavailable";

const MESSAGES: Record<CampaignErrorStatus, string> = {
  duplicate:
    "Yêu cầu tạo này đã được dùng với nội dung khác. Tải lại trang để tạo yêu cầu mới.",
  invalid: "Dữ liệu không hợp lệ; chưa có thay đổi nào được lưu.",
  not_found: "Không tìm thấy chiến dịch hoặc tin đăng này.",
  forbidden: "Bạn không có quyền thực hiện thao tác này.",
  listing_unavailable:
    "Chưa kiểm tra được tin đăng vì dịch vụ tin đăng đang tạm ngưng. Chưa có chiến dịch nào được tạo; vui lòng thử lại sau.",
  audit_unavailable:
    "Hệ thống kiểm toán đang tạm ngưng nên chưa có thay đổi nào được thực hiện. Vui lòng thử lại sau.",
  promotion_unavailable:
    "Dịch vụ khuyến mãi đang tạm ngưng nên chưa có thay đổi nào được thực hiện. Vui lòng thử lại sau.",
  timeout:
    "Chưa rõ thao tác đã được thực hiện hay chưa. Hãy kiểm tra danh sách chiến dịch trước khi thử lại.",
  unavailable:
    "Không thực hiện được thao tác. Kiểm tra danh sách chiến dịch rồi thử lại sau.",
};

export function campaignErrorMessage(status: string): string {
  return MESSAGES[status as CampaignErrorStatus] ?? MESSAGES.unavailable;
}
