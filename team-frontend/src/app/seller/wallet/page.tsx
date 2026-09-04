import { redirect } from "next/navigation";

import { formatPrice } from "@/components/ui/format";
import { WalletPayoutButton } from "@/features/seller/WalletPayoutButton";
import {
  getWalletBalance,
  listLedgerEntries,
} from "@/lib/gateway/payment";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerWalletPage() {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const [balance, entries] = await Promise.all([
    getWalletBalance(me.id),
    listLedgerEntries(me.id),
  ]);

  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 rounded-2xl border border-slate-100 bg-white p-6 shadow-sm sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-black text-slate-900">💰 Ví Người Bán</h1>
          <p className="mt-1 text-sm text-slate-500">
            Số dư khả dụng và lịch sử giao dịch ví.
          </p>
          <div className="mt-3 text-3xl font-black text-emerald-600">
            {formatPrice(balance, "VND")}
          </div>
        </div>
        <WalletPayoutButton sellerId={me.id} balance={balance} />
      </div>

      <div className="overflow-hidden rounded-2xl border border-slate-100 bg-white shadow-sm">
        <div className="border-b border-slate-100 bg-slate-50/70 px-5 py-3 text-xs font-semibold text-slate-700">
          Lịch sử giao dịch ({entries.length})
        </div>
        {entries.length === 0 ? (
          <div className="p-8 text-center text-xs text-slate-400">
            Chưa có giao dịch nào.
          </div>
        ) : (
          <table className="w-full text-left text-xs">
            <thead className="border-b border-slate-100 bg-slate-50/50 text-[11px] font-semibold uppercase tracking-wider text-slate-500">
              <tr>
                <th className="px-5 py-3">Loại</th>
                <th className="px-5 py-3">Số tiền</th>
                <th className="px-5 py-3">Trạng thái</th>
                <th className="px-5 py-3">Ngày</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {entries.map((e) => (
                <tr key={e.id} className="hover:bg-slate-50/60">
                  <td className="px-5 py-3 font-medium text-slate-800">
                    {e.type || "—"}
                  </td>
                  <td className="px-5 py-3 font-bold text-slate-900">
                    {formatPrice(e.amount, "VND")}
                  </td>
                  <td className="px-5 py-3 text-slate-600">{e.status || "—"}</td>
                  <td className="px-5 py-3 text-slate-500">{e.createdAt}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
