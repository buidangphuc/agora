import { Link } from "@/components/ui/Link";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { notFound } from "next/navigation";
import type { ReactNode } from "react";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { PriceTag } from "@/components/ui/PriceTag";
import { Result } from "@/components/ui/Result";
import {
  PaymentSimulator,
  type SimulatorPhase,
} from "@/features/payment/PaymentSimulator";
import {
  CancelReason,
  OrderStatus,
} from "@/generated/platform/order/v1/order_pb.js";
import {
  PaymentMethod,
  PaymentStatus,
} from "@/generated/platform/payment/v1/payment_pb.js";
import { listBuyerOrdersResult } from "@/lib/gateway/orders";
import {
  type ViewPaymentTransaction,
  createPayment,
  getPayment,
} from "@/lib/gateway/payment";
import { getPrincipal } from "@/lib/gateway/session";

import { loadOrder } from "./data";

export const dynamic = "force-dynamic";

const inflightTransactions = new Map<string, Promise<ViewPaymentTransaction>>();

function ensureTransaction(
  orderId: string,
  method: PaymentMethod,
): Promise<ViewPaymentTransaction> {
  const pending = inflightTransactions.get(orderId);
  if (pending) return pending;
  const created = (async () =>
    (await getPayment(undefined, orderId)) ??
    (await createPayment(orderId, method)).transaction)().finally(() => {
    inflightTransactions.delete(orderId);
  });
  inflightTransactions.set(orderId, created);
  return created;
}

export default async function MockPaymentPage(props: {
  params: Promise<{ id: string }>;
}) {
  const params = await props.params;
  const me = getPrincipal();
  if (!me)
    redirectGuestToLogin(`/checkout/pay/${encodeURIComponent(params.id)}`);

  const order = await loadOrder(params.id);
  if (!order) notFound();

  const accountLink = (href: string) => (
    <Link href={href} variant="primary">
      Xem đơn hàng
    </Link>
  );
  const due = order.paymentDueAt ? Date.parse(order.paymentDueAt) : null;
  const expired =
    order.status === OrderStatus.PENDING &&
    due !== null &&
    !Number.isNaN(due) &&
    Date.now() >= due;

  let state: ReactNode = null;
  if (order.status !== OrderStatus.PENDING) {
    if (order.status === OrderStatus.CANCELLED) {
      state =
        order.cancelReason === CancelReason.PAYMENT_EXPIRED ? (
          <Result
            status="warning"
            title="Đơn hàng đã hết hạn thanh toán"
            extra={accountLink("/account/orders")}
          />
        ) : (
          <Result
            status="warning"
            title="Đơn hàng đã bị hủy"
            subTitle="Đơn hàng đã bị hủy và tồn kho đã được giải phóng."
            extra={accountLink("/account/orders")}
          />
        );
    } else {
      state = (
        <Result
          status="success"
          title="Đơn hàng đã được thanh toán"
          extra={accountLink(`/account/orders/${order.id}`)}
        />
      );
    }
  } else if (order.paymentMethod === PaymentMethod.COD) {
    state = (
      <Result
        status="info"
        title="Đơn hàng thanh toán khi nhận hàng"
        extra={accountLink(`/account/orders/${order.id}`)}
      />
    );
  } else if (expired) {
    state = (
      <Result
        status="warning"
        title="Đơn hàng đã hết hạn thanh toán"
        extra={accountLink("/account/orders")}
      />
    );
  } else {
    const transaction = await ensureTransaction(params.id, order.paymentMethod);
    let nextOrderId: string | undefined;
    if (order.checkoutId) {
      const siblings = await listBuyerOrdersResult();
      if (siblings.ok) {
        nextOrderId = siblings.orders
          .filter(
            (o) =>
              o.checkoutId === order.checkoutId &&
              o.id !== order.id &&
              o.status === OrderStatus.PENDING &&
              o.paymentMethod !== PaymentMethod.COD,
          )
          .sort((a, b) => a.sellerId.localeCompare(b.sellerId))[0]?.id;
      }
    }
    const phase: SimulatorPhase =
      transaction.status === PaymentStatus.PAID
        ? "success"
        : transaction.status === PaymentStatus.FAILED
          ? "failed"
          : "idle";
    state = (
      <>
        {phase === "failed" && due !== null && !Number.isNaN(due) ? (
          <Alert
            type="info"
            description={`Đơn hàng vẫn được giữ đến ${new Date(due).toLocaleString("vi-VN")}. Bạn có thể thử thanh toán lại.`}
          />
        ) : null}
        <PaymentSimulator
          orderId={order.id}
          transactionId={transaction.id}
          initialPhase={phase}
          nextOrderId={nextOrderId}
        />
      </>
    );
  }

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
                children: <PriceTag price={order.totalAmount} size="lg" />,
              },
              {
                key: "method",
                label: "Phương thức",
                children: order.paymentMethodText,
              },
            ]}
          />
        </CardContent>
      </Card>

      {state}
    </section>
  );
}
