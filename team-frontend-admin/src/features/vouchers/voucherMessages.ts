export const VOUCHER_MESSAGES = {
  forbidden: "Bạn không có quyền thực hiện thao tác này trên voucher sàn.",
  fields: "Thông tin voucher chưa hợp lệ; kiểm tra các trường được đánh dấu.",
  invalid:
    "Dịch vụ khuyến mãi từ chối dữ liệu voucher. Chưa có voucher nào được tạo.",
  invalidChange:
    "Dịch vụ khuyến mãi từ chối thay đổi này. Voucher chưa có thay đổi nào.",
  auditDown:
    "Chưa tạo được voucher: hệ thống kiểm toán đang tạm ngưng. Thao tác trên toàn sàn phải được ghi kiểm toán, nên chưa có voucher nào được tạo. Vui lòng thử lại sau.",
  promotionDown:
    "Chưa tạo được voucher: dịch vụ khuyến mãi đang tạm ngưng nên chưa có voucher nào được tạo. Vui lòng thử lại sau.",
  auditDownChange:
    "Chưa thay đổi được voucher: hệ thống kiểm toán đang tạm ngưng. Thao tác trên toàn sàn phải được ghi kiểm toán, nên chưa có thay đổi nào. Vui lòng thử lại sau.",
  promotionDownChange:
    "Chưa thay đổi được voucher: dịch vụ khuyến mãi đang tạm ngưng nên chưa có thay đổi nào. Vui lòng thử lại sau.",
  timeout:
    "Hệ thống không phản hồi kịp nên chưa rõ thao tác đã được thực hiện hay chưa. Kiểm tra voucher trong danh sách trước khi thử lại.",
  duplicate: "Mã này đã được dùng cho voucher khác. Hãy đổi mã khác.",
  failed: "Không tạo được voucher. Kiểm tra danh sách hoặc đổi mã rồi thử lại.",
  failedChange:
    "Không thay đổi được voucher. Tải lại trang để xem trạng thái hiện tại rồi thử lại.",
} as const;
