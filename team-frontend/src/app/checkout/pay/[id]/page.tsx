import { notFound, redirect } from "next/navigation";

import { MockPaymentView } from "@/features/payment/MockPaymentView";
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

  return (
    <section className="py-6">
      <MockPaymentView order={order} transaction={transaction} />
    </section>
  );
}
