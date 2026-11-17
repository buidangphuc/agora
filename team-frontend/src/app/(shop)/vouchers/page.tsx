import { MyVouchersSection } from "@/features/voucher/MyVouchersSection";
import { VouchersView } from "@/features/voucher/VouchersView";
import { parseVoucherTab } from "@/features/voucher/tabs";
import { listMyVouchers, listVouchers } from "@/lib/gateway/promotion";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Kho Voucher & Mã Giảm Giá",
  description: "Xem các mã giảm giá và voucher đang có trên Marketplace.",
};

export default async function VouchersPage(props: {
  searchParams: Promise<{ type?: string | string[] }>;
}) {
  const searchParams = await props.searchParams;
  // Real vouchers from team-promotion via the gateway (server-only). Empty on
  // any outage so the page still renders.
  const [vouchers, mine] = await Promise.all([
    listVouchers(),
    listMyVouchers(),
  ]);
  const type = parseVoucherTab(searchParams.type);

  return (
    <section className="space-y-8 py-2">
      {mine.ok && <MyVouchersSection vouchers={mine.vouchers} />}
      <VouchersView vouchers={vouchers} type={type} />
    </section>
  );
}
