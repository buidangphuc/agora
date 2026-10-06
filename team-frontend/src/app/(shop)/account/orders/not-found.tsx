import Link from "next/link";

import { Result } from "@/components/ui/Result";
import { linkButton } from "@/features/order/linkStyles";

export default function OrderNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy đơn hàng"
      subTitle="Đơn hàng không tồn tại hoặc đã bị xóa."
      extra={
        <Link href="/account/orders" className={linkButton.outlineMd}>
          Về đơn hàng của tôi
        </Link>
      }
    />
  );
}
