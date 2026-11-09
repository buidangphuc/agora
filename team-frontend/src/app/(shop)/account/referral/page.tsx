import { redirectGuestToLogin } from "@/features/auth/guardPath";

import { Badge } from "@/components/ui/Badge";
import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Statistic } from "@/components/ui/Statistic";
import { Timeline } from "@/components/ui/Timeline";
import { formatXu } from "@/components/ui/format";
import { AccountShell } from "@/features/account/AccountShell";
import { CopyCodeButton } from "@/features/account/referral/CopyCodeButton";
import { GenerateReferralCodeButton } from "@/features/account/referral/GenerateReferralCodeButton";
import { RedeemReferralForm } from "@/features/account/referral/RedeemReferralForm";
import { ShareInviteButton } from "@/features/account/referral/ShareInviteButton";
import { getMyReferral, listReferralRewards } from "@/lib/gateway/referral";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Mời bạn bè",
};

const STATUS = {
  PENDING: { label: "Đang chờ", variant: "warning", tone: "warning" },
  RELEASED: { label: "Đã nhận", variant: "success", tone: "success" },
  VOID: { label: "Đã hủy", variant: "neutral", tone: "neutral" },
} as const;

function SectionTitle({ children }: { children: string }) {
  return (
    <CardHeader>
      <h2 className="text-base font-semibold text-text-primary">{children}</h2>
    </CardHeader>
  );
}

export default async function ReferralPage() {
  if (!getPrincipal()) redirectGuestToLogin("/account/referral");

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
        <CardContent>
          <div className="space-y-4">
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
                      <ShareInviteButton code={referral.code} />
                    </span>
                  ) : (
                    <span className="flex flex-wrap items-center gap-3">
                      <span className="font-normal text-text-secondary">
                        Bạn chưa có mã giới thiệu.
                      </span>
                      <GenerateReferralCodeButton />
                      <ShareInviteButton code="" />
                    </span>
                  ),
                },
              ]}
            />
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <Statistic title="Đã mời" value={referral.invitedCount} />
              <Statistic
                title="Xu đang chờ"
                value={
                  <span className="text-action-primary">
                    {formatXu(referral.pendingXu)}
                  </span>
                }
              />
              <Statistic
                title="Xu đã nhận"
                value={
                  <span className="text-action-primary">
                    {formatXu(referral.releasedXu)}
                  </span>
                }
              />
            </div>
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
              tone: STATUS[r.status].tone,
              description: (
                <span className="flex flex-wrap items-center gap-2">
                  <span
                    className={
                      r.status === "VOID"
                        ? "font-semibold text-text-secondary line-through"
                        : "font-semibold text-action-primary"
                    }
                  >
                    +{formatXu(r.amount)}
                  </span>
                  <Badge variant={STATUS[r.status].variant}>
                    {STATUS[r.status].label}
                  </Badge>
                  {r.status === "PENDING" && r.releaseAt ? (
                    <span className="text-sm text-text-secondary">
                      Dự kiến nhận: {r.releaseAt}
                    </span>
                  ) : null}
                </span>
              ),
            }))}
          />
        </CardContent>
      </Card>
    </AccountShell>
  );
}
