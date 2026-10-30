import { Link } from "@/components/ui/Link";

/** Reviews are written from a completed order, so point viewers there. */
export function ReviewOrdersHint() {
  return (
    <p className="text-sm text-text-secondary">
      Để đánh giá, hãy vào{" "}
      <span className="font-medium">
        <Link href="/account/orders">Đơn hàng của tôi</Link>
      </span>{" "}
      và chọn sản phẩm đã mua.
    </p>
  );
}
