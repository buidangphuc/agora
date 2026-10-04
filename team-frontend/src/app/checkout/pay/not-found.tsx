import Link from "next/link";

import { Result } from "@/components/ui/Result";
import { linkButtonPrimary } from "@/features/cart/linkButton";

export default function PaymentNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy giao dịch"
      subTitle="Đơn hàng hoặc giao dịch thanh toán này không tồn tại."
      extra={
        <Link href="/account/orders" className={linkButtonPrimary}>
          Xem đơn hàng
        </Link>
      }
    />
  );
}
