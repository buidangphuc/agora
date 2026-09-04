import { VoucherManager } from "@/features/voucher/VoucherManager";
import { VouchersView } from "@/features/voucher/VouchersView";
import { listVouchers } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Kho Voucher & Mã Giảm Giá Marketplace — Siêu Khuyến Mãi",
  description: "Săn mã giảm giá, voucher Freeship 0Đ và hoàn xu 20% mỗi ngày.",
};

export default async function VouchersPage() {
  // Real vouchers from team-promotion via the gateway (server-only). Empty on
  // any outage so the page still renders.
  const vouchers = await listVouchers();

  // Creating vouchers is a seller-only action (backend also rejects anonymous
  // in team-promotion). Only sellers see the manager; everyone can browse/claim.
  const canManage = Boolean(getPrincipal()) && hasScope("listing.write");

  return (
    <section className="space-y-8 py-2">
      {canManage && <VoucherManager initialVouchers={vouchers} />}
      <VouchersView />
    </section>
  );
}
