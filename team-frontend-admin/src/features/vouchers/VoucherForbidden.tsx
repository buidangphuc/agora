import { LinkButton } from "@/components/LinkButton";
import { Result } from "@/components/ui/Result";

export const GATEWAY_DENIED =
  "Gateway từ chối quyền admin cho phiên đăng nhập này.";
export const NO_ADMIN_SCOPE =
  "Tài khoản của bạn không có quyền admin để quản lý voucher sàn.";

export function VoucherForbidden({ subTitle }: { subTitle: string }) {
  return (
    <Result
      status="403"
      title="Cần tài khoản Quản trị"
      subTitle={subTitle}
      extra={<LinkButton href="/">Quay lại trang chủ</LinkButton>}
    />
  );
}
