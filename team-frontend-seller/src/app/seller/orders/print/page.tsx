import { Alert } from "@/components/ui/Alert";
import { redirectGuestToLogin } from "@/features/auth/guardPath";
import { OrderDetailView } from "@/features/seller/OrderDetailView";
import { PrintButton } from "@/features/seller/PrintButton";
import { PrintStyles } from "@/features/seller/PrintStyles";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { parseIds, runPool } from "@/features/seller/bulkShip";
import { type ViewOrder, getOrder } from "@/lib/gateway/orders";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

import { isOwnedBy } from "../[id]/data";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "In phiếu đóng gói",
  robots: { index: false, follow: false },
};

async function readOrder(id: string): Promise<ViewOrder | null> {
  try {
    return await getOrder(id);
  } catch {
    return null;
  }
}

export default async function PrintOrdersPage(props: {
  searchParams?: Promise<{ ids?: string | string[] }>;
}) {
  const searchParams = (await props.searchParams) ?? {};
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/orders/print", searchParams);
  if (!hasScope("listing.write")) return null;

  const raw = Array.isArray(searchParams.ids)
    ? searchParams.ids.join(",")
    : searchParams.ids;
  const parsed = parseIds(raw);
  if (!parsed.ok) {
    return (
      <div data-print-hidden>
        <Alert type="error" description={parsed.error} />
      </div>
    );
  }

  const results = await runPool(parsed.ids, readOrder);
  const orders: ViewOrder[] = [];
  const skipped: string[] = [];
  results.forEach((r, i) => {
    const o = r.done ? r.value : null;
    if (o && isOwnedBy(o, me)) orders.push(o);
    else skipped.push(parsed.ids[i]);
  });

  return (
    <>
      <PrintStyles />
      <SellerPageHeader
        title="In phiếu đóng gói"
        description={`${orders.length} phiếu sẵn sàng in.`}
        trail={[{ label: "Quản lý đơn hàng", href: "/seller/orders" }]}
        action={<PrintButton />}
      />
      {skipped.length > 0 && (
        <div data-print-hidden>
          <Alert
            type="warning"
            description={`Không in được: ${skipped.join(", ")}`}
          />
        </div>
      )}
      {orders.map((order) => (
        <div key={order.id} style={{ breakAfter: "page" }}>
          <OrderDetailView
            order={order}
            shipment={null}
            tab="items"
            tabHref={() => "#"}
          />
        </div>
      ))}
    </>
  );
}
