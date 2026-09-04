import { redirect } from "next/navigation";

import { formatPrice } from "@/components/ui/format";
import { SubscribeButton } from "@/features/seller/SubscribeButton";
import { getEntitlements, listPlans } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export default async function SellerPlansPage() {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const [plans, entitlements] = await Promise.all([
    listPlans(),
    getEntitlements(me.id),
  ]);

  return (
    <div className="space-y-6">
      <div className="rounded-2xl border border-slate-100 bg-white p-6 shadow-sm">
        <h1 className="text-xl font-black text-slate-900">
          ⭐ Gói Đăng Ký Người Bán
        </h1>
        <p className="mt-1 text-sm text-slate-500">
          Gói hiện tại:{" "}
          <span className="font-bold text-brand">{entitlements.tierText}</span>
        </p>
      </div>

      {plans.length === 0 ? (
        <div className="rounded-2xl border border-slate-100 bg-white p-8 text-center text-xs text-slate-400 shadow-sm">
          Chưa có gói đăng ký nào.
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {plans.map((plan) => {
            const current = plan.tier === entitlements.tier;
            return (
              <div
                key={plan.id}
                className={`flex flex-col rounded-2xl border bg-white p-5 shadow-sm ${
                  current ? "border-brand" : "border-slate-100"
                }`}
              >
                <h2 className="text-base font-bold text-slate-900">
                  {plan.tierText}
                </h2>
                <div className="mt-1 text-2xl font-black text-slate-900">
                  {formatPrice(plan.price, "VND")}
                </div>
                <ul className="mt-3 flex-1 space-y-1.5 text-xs text-slate-600">
                  {plan.features.map((f) => (
                    <li key={f} className="flex items-start gap-1.5">
                      <span className="text-emerald-500">✓</span>
                      <span>{f}</span>
                    </li>
                  ))}
                </ul>
                <div className="mt-4">
                  <SubscribeButton planId={plan.id} current={current} />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
