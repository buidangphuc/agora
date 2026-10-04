import Link from "next/link";
import { notFound, redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { PriceTag } from "@/components/ui/PriceTag";
import { Result } from "@/components/ui/Result";
import { linkButtonPrimary } from "@/features/cart/linkButton";
import {
  PaymentSimulator,
  type SimulatorPhase,
} from "@/features/payment/PaymentSimulator";
import { OrderStatus } from "@/generated/platform/order/v1/order_pb.js";
import { PaymentStatus } from "@/generated/platform/payment/v1/payment_pb.js";
import { getOrder } from "@/lib/gateway/orders";
import { getPayment } from "@/lib/gateway/payment";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function MockPaymentPage({
  params,
}: {
  params: { id: string };
}) {
  const me = getPrincipal();
  if (!me) redirect("/login");

  const order = await getOrder(params.id);
  if (!order) notFound();

  const transaction = await getPayment(undefined, params.id);
  if (!transaction) notFound();

  const cancelled = order.status === OrderStatus.CANCELLED;
  const phase: SimulatorPhase =
    transaction.status === PaymentStatus.PAID
      ? "success"
      : transaction.status === PaymentStatus.FAILED
        ? "failed"
        : "idle";

  return (
    <section className="mx-auto max-w-xl space-y-4 py-4">
      <Card>
        <CardHeader>
          <CardTitle>Thanh toán đơn hàng</CardTitle>
        </CardHeader>
        <CardContent>
          <Descriptions
            column={1}
            items={[
              {
                key: "order",
                label: "Mã đơn hàng",
                children: `#${order.id.slice(0, 8)}`,
              },
              {
                key: "amount",
                label: "Số tiền",
                children: <PriceTag price={transaction.amount} size="lg" />,
              },
              {
                key: "method",
                label: "Phương thức",
                children: transaction.methodText,
              },
            ]}
          />
        </CardContent>
      </Card>

      {cancelled ? (
        <Result
          status="warning"
          title="Đơn hàng đã bị hủy"
          extra={
            <Link href="/account/orders" className={linkButtonPrimary}>
              Xem đơn hàng
            </Link>
          }
        >
          <Alert
            type="warning"
            description="Thanh toán không thành công nên hệ thống đã hoàn tác: tồn kho đã được giải phóng và đơn hàng đã bị hủy."
          />
        </Result>
      ) : (
        <PaymentSimulator
          orderId={order.id}
          transactionId={transaction.id}
          initialPhase={phase}
        />
      )}
    </section>
  );
}
