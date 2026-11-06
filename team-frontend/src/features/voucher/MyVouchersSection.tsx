import type { ViewVoucher } from "@/lib/gateway/promotion";
import { VoucherCard } from "./VoucherCard";

export function MyVouchersSection({ vouchers }: { vouchers: ViewVoucher[] }) {
  if (vouchers.length === 0) return null;
  return (
    <section className="space-y-4" aria-labelledby="my-vouchers-heading">
      <h2
        id="my-vouchers-heading"
        className="text-lg font-semibold text-text-primary"
      >
        Voucher của bạn
      </h2>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {vouchers.map((v) => (
          <VoucherCard key={v.id} voucher={v} />
        ))}
      </div>
    </section>
  );
}
