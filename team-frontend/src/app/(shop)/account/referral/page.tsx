import { redirect } from "next/navigation";

import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Statistic } from "@/components/ui/Statistic";
import { Timeline } from "@/components/ui/Timeline";
import { formatPrice } from "@/components/ui/format";
import { AccountShell } from "@/features/account/AccountShell";
import { CopyCodeButton } from "@/features/account/referral/CopyCodeButton";
import { GenerateReferralCodeButton } from "@/features/account/referral/GenerateReferralCodeButton";
import { RedeemReferralForm } from "@/features/account/referral/RedeemReferralForm";
import { getMyReferral, listReferralRewards } from "@/lib/gateway/referral";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Mời bạn bè | Marketplace",
};

function SectionTitle({ children }: { children: string }) {
  return (
    <CardHeader>
      <h2 className="text-base font-semibold text-text-primary">{children}</h2>
    </CardHeader>
  );
}

export default async function ReferralPage() {
  if (!getPrincipal()) redirect("/login");

  const [referral, rewards] = await Promise.all([
    getMyReferral(),
    listReferralRewards(),
  ]);

  return (
    <AccountShell
      current="referral"
      title="Mời bạn bè"
      description="Chia sẻ mã giới thiệu của bạn để nhận thưởng khi bạn bè tham gia."
    >
      <Card>
        <SectionTitle>Mã giới thiệu của bạn</SectionTitle>
        <CardContent className="space-y-4">
          <Descriptions
            column={1}
            items={[
              {
                key: "code",
                label: "Mã giới thiệu",
                children: referral.code ? (
                  <span className="flex flex-wrap items-center gap-3">
                    <span
                      data-testid="referral-code"
                      className="font-mono text-xl font-bold tracking-wider text-action-primary"
                    >
                      {referral.code}
                    </span>
                    <CopyCodeButton code={referral.code} />
                  </span>
                ) : (
                  <span className="flex flex-wrap items-center gap-3">
                    <span className="font-normal text-text-secondary">
                      Bạn chưa có mã giới thiệu.
                    </span>
                    <GenerateReferralCodeButton />
                  </span>
                ),
              },
            ]}
          />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Statistic title="Đã mời" value={referral.invitedCount} />
            <Statistic
              title="Tổng thưởng"
              value={
                <span className="text-action-primary">
                  {formatPrice(referral.rewardsTotal)}
                </span>
              }
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <SectionTitle>Nhập mã của bạn bè</SectionTitle>
        <CardContent>
          <RedeemReferralForm />
        </CardContent>
      </Card>

      <Card>
        <SectionTitle>Lịch sử thưởng</SectionTitle>
        <CardContent>
          <Timeline
            emptyText="Chưa có phần thưởng nào."
            items={rewards.map((r) => ({
              key: r.id,
              title: r.reason || "Phần thưởng giới thiệu",
              time: r.createdAt,
              tone: "success",
              description: (
                <span className="font-semibold text-action-primary">
                  +{formatPrice(r.amount)}
                </span>
              ),
            }))}
          />
        </CardContent>
      </Card>
    </AccountShell>
  );
}
