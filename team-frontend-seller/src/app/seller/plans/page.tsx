import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { Empty } from "@/components/ui/Empty";
import { Tag } from "@/components/ui/Tag";
import { formatPrice } from "@/components/ui/format";
import { CancelSubscriptionButton } from "@/features/seller/CancelSubscriptionButton";
import { SellerPageHeader } from "@/features/seller/SellerPageHeader";
import { SubscribeButton } from "@/features/seller/SubscribeButton";
import { formatPeriodEnd } from "@/features/seller/periodEnd";
import {
  getEntitlements,
  getMySubscription,
  listPlans,
} from "@/lib/gateway/promotion";
import { getPrincipal, hasScope } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = { title: "Gói đăng ký" };

/** Plan Card grid; the current plan carries a "Gói hiện tại" Tag and a disabled button. */
export default async function SellerPlansPage() {
  const me = getPrincipal();
  if (!me) redirectGuestToLogin("/seller/plans");
  if (!hasScope("listing.write")) return null;

  const [plans, entitlements, subResult] = await Promise.all([
    listPlans(),
    getEntitlements(me.id),
    getMySubscription().then(
      (value) => ({ ok: true as const, value }),
      () => ({ ok: false as const }),
    ),
  ]);
  const sub = subResult.ok ? subResult.value : null;
  const readFailed = !subResult.ok;
  const currentTier = sub ? sub.tier : entitlements.tier;

  return (
    <>
      <SellerPageHeader
        title="Gói đăng ký"
        description={`Gói hiện tại: ${entitlements.tierText}`}
      />

      {readFailed && (
        <Alert
          type="error"
          description="Không tải được gói hiện tại. Thử lại sau."
        />
      )}

      {plans.length === 0 ? (
        <Alert type="info" description="Chưa có gói đăng ký nào để chọn." />
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {plans.map((plan) => {
            const current = !readFailed && plan.tier === currentTier;
            const paidCurrent = current && !!sub && plan.tier > 1;
            return (
              <div
                key={plan.id}
                className={
                  current
                    ? "grid h-full rounded-2xl ring-2 ring-action-primary"
                    : "grid h-full"
                }
              >
                <Card>
                  <CardContent>
                    <div className="flex h-full flex-col gap-3">
                      <div className="flex items-center justify-between gap-2">
                        <h2 className="text-base font-semibold text-text-primary">
                          {plan.tierText}
                        </h2>
                        {current && <Tag color="primary">Gói hiện tại</Tag>}
                      </div>
                      <p className="text-2xl font-bold text-action-primary">
                        {formatPrice(plan.price)} / {plan.termDays} ngày
                      </p>
                      {current && entitlements.limits.max_listings && (
                        <p className="text-sm text-text-secondary">
                          Số tin tối đa: {entitlements.limits.max_listings}
                        </p>
                      )}
                      {paidCurrent && sub && (
                        <div className="space-y-1 text-sm text-text-secondary">
                          <p>
                            Hết hạn ngày {formatPeriodEnd(sub.currentPeriodEnd)}
                          </p>
                          <p>
                            Tự động gia hạn: {sub.autoRenew ? "Bật" : "Tắt"}
                          </p>
                        </div>
                      )}
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
                      {readFailed ? (
                        <Button
                          variant="outline"
                          fullWidth
                          disabled
                          aria-disabled="true"
                        >
                          Đăng ký
                        </Button>
                      ) : (
                        <SubscribeButton
                          planId={plan.id}
                          price={plan.price}
                          termDays={plan.termDays}
                          direction={
                            current
                              ? "current"
                              : plan.tier > currentTier
                                ? "upgrade"
                                : "downgrade"
                          }
                          periodEnd={sub?.currentPeriodEnd}
                        />
                      )}
                      {paidCurrent && sub && (
                        <CancelSubscriptionButton
                          paid
                          autoRenew={sub.autoRenew}
                          periodEnd={sub.currentPeriodEnd}
                        />
                      )}
                    </div>
                  </CardContent>
                </Card>
              </div>
            );
          })}
        </div>
      )}
    </>
  );
}
