import Link from "next/link";

import { Card } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tabs } from "@/components/ui/Tabs";
import { linkButtonClass } from "@/features/home/linkButton";
import type { ViewVoucher } from "@/lib/gateway/promotion";
import { VoucherCard } from "./VoucherCard";
import {
  VOUCHER_TABS,
  type VoucherTab,
  inTab,
  parseVoucherTab,
  tabCounts,
  voucherTabHref,
} from "./tabs";

/**
 * The voucher hub body: banner, URL-driven tabs with counts and a grid of the
 * real vouchers the server fetched. Server component, no client state.
 */
export function VouchersView({
  vouchers,
  type = "all",
}: {
  vouchers: ViewVoucher[];
  type?: VoucherTab;
}) {
  const counts = tabCounts(vouchers);
  const shown = vouchers.filter((v) => inTab(v, type));

  return (
    <div className="space-y-6">
      <section className="rounded-2xl bg-gradient-to-r from-primary-500 to-primary-700 p-6 text-text-inverse shadow-preline-card sm:p-8">
        <h1 className="text-lg font-bold sm:text-2xl">Kho Voucher</h1>
        <p className="mt-2 max-w-xl text-sm text-primary-100">
          Xem các mã giảm giá đang có và nhập mã khi thanh toán.
        </p>
      </section>

      <div className="max-w-full overflow-x-auto">
        <Tabs
          items={VOUCHER_TABS.map((t) => ({
            id: t.id,
            label: t.label,
            badge: counts[t.id],
          }))}
          activeId={type}
          hrefFor={(id) => voucherTabHref(parseVoucherTab(id))}
        />
      </div>

      {shown.length === 0 ? (
        <Card>
          <Empty
            description="Chưa có voucher nào trong mục này."
            action={
              <Link href="/search" className={linkButtonClass("primary")}>
                Xem sản phẩm
              </Link>
            }
          />
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {shown.map((v) => (
            <VoucherCard key={v.id || v.code} voucher={v} />
          ))}
        </div>
      )}
    </div>
  );
}
