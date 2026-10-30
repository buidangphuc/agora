import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";

export default function OrderNotFound() {
  return (
    <Result
      status="404"
      title="Không tìm thấy đơn hàng"
      subTitle="Đơn hàng không tồn tại hoặc đã bị xóa."
      extra={
        <Link href="/account/orders" variant="outline" size="md">
          Về đơn hàng của tôi
        </Link>
      }
    />
  );
}
