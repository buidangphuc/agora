import Link from "next/link";

/** Reviews are written from a completed order, so point viewers there. */
export function ReviewOrdersHint() {
  return (
    <p className="text-sm text-text-secondary">
      Để đánh giá, hãy vào{" "}
      <Link
        href="/account/orders"
        className="font-medium text-text-primary underline"
      >
        Đơn hàng của tôi
      </Link>{" "}
      và chọn sản phẩm đã mua.
    </p>
  );
}
