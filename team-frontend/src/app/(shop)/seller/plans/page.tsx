import { redirect } from "next/navigation";

import { Alert } from "@/components/ui/Alert";
import { Card, CardContent } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { SubscribeButton } from "@/features/seller/SubscribeButton";
import { getEntitlements, listPlans } from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Gói đăng ký | Kênh người bán" };

/** Plan Card grid; the current plan carries a "Gói hiện tại" Tag and a disabled button. */
export default async function SellerPlansPage() {
  const me = getPrincipal();
  if (!me || !hasScope("listing.write")) redirect("/login");

  const [plans, entitlements] = await Promise.all([
    listPlans(),
    getEntitlements(me.id),
  ]);

  return (
    <>
      <SellerPageHeader
        title="Gói đăng ký"
        description={`Gói hiện tại: ${entitlements.tierText}`}
      />

      {plans.length === 0 ? (
        <Alert type="info" description="Chưa có gói đăng ký nào để chọn." />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {plans.map((plan) => {
            const current = plan.tier === entitlements.tier;
            return (
              <Card
                key={plan.id}
                className={current ? "border-action-primary" : ""}
              >
                <CardContent className="flex h-full flex-col gap-3">
                  <div className="flex items-center justify-between gap-2">
                    <h2 className="text-base font-semibold text-text-primary">
                      {plan.tierText}
                    </h2>
                    {current && <Tag color="primary">Gói hiện tại</Tag>}
                  </div>
                  <p className="text-2xl font-bold text-action-primary">
                    {formatPrice(plan.price)}
                  </p>
                  {plan.features.length === 0 ? (
                    <Empty description="Chưa có mô tả quyền lợi" />
                  ) : (
                    <ul className="flex-1 space-y-1.5 text-sm text-text-secondary">
                      {plan.features.map((f) => (
                        <li key={f} className="flex items-start gap-2">
                          <span aria-hidden="true" className="text-success">
                            ✓
                          </span>
                          <span>{f}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                  <SubscribeButton planId={plan.id} current={current} />
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}
    </>
  );
}
