import { VoucherManager } from "@/features/voucher/VoucherManager";
import { VouchersView } from "@/features/voucher/VouchersView";
import { parseVoucherTab } from "@/features/voucher/tabs";
import { listVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Kho Voucher & Mã Giảm Giá",
  description: "Xem các mã giảm giá và voucher đang có trên Marketplace.",
};

export default async function VouchersPage({
  searchParams,
}: {
  searchParams: { type?: string | string[] };
}) {
  // Real vouchers from team-promotion via the gateway (server-only). Empty on
  // any outage so the page still renders.
  const vouchers = await listVouchers();
  const type = parseVoucherTab(searchParams.type);

  // Creating vouchers is a seller-only action (backend also rejects anonymous
  // in team-promotion). Only sellers see the manager.
  const canManage = Boolean(getPrincipal()) && hasScope("listing.write");

  return (
    <section className="space-y-8 py-2">
      {canManage && <VoucherManager initialVouchers={vouchers} />}
      <VouchersView vouchers={vouchers} type={type} />
    </section>
  );
}
