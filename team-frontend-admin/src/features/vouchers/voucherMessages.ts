export const VOUCHER_MESSAGES = {
  forbidden: "Bạn không có quyền tạo voucher sàn.",
  fields: "Thông tin voucher chưa hợp lệ; kiểm tra các trường được đánh dấu.",
  invalid:
    "Dịch vụ khuyến mãi từ chối dữ liệu voucher. Chưa có voucher nào được tạo.",
  auditDown:
    "Chưa tạo được voucher: hệ thống kiểm toán hoặc dịch vụ khuyến mãi đang tạm ngưng. Thao tác trên toàn sàn phải được ghi kiểm toán, nên chưa có voucher nào được tạo. Vui lòng thử lại sau.",
  timeout:
    "Hệ thống không phản hồi kịp nên chưa rõ voucher đã được tạo hay chưa. Kiểm tra danh sách voucher trước khi thử lại.",
  failed:
    "Không tạo được voucher. Mã có thể đã được dùng cho voucher khác; kiểm tra danh sách hoặc đổi mã rồi thử lại.",
} as const;
