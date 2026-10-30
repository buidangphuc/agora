import { Link } from "@/components/ui/Link";
import { Result } from "@/components/ui/Result";

export default function PaymentNotFound() {
  return (
    <section className="mx-auto w-full max-w-xl py-6 md:py-10">
      <Result
        status="404"
        title="Không tìm thấy giao dịch"
        subTitle="Đơn hàng hoặc giao dịch thanh toán này không tồn tại."
        extra={
          <Link href="/account/orders" variant="primary">
            Xem đơn hàng
          </Link>
        }
      />
    </section>
  );
}
