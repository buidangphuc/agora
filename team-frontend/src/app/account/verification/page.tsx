import { redirect } from "next/navigation";

import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Tag } from "@/components/ui/Tag";
import { AccountShell } from "@/features/account/AccountShell";
import { SubmitKycForm } from "@/features/account/verification/SubmitKycForm";
import { statusTone } from "@/features/account/verification/status";
import { getPrincipal } from "@/lib/gateway/session";
import { getVerificationStatus } from "@/lib/gateway/verification";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Xác minh tài khoản | Marketplace",
};

export default async function VerificationPage() {
  if (!getPrincipal()) redirect("/login");

  const verification = await getVerificationStatus();

  const items = [
    {
      key: "status",
      label: "Trạng thái",
      children: (
        <Tag color={statusTone(verification.status)}>
          {verification.statusText}
        </Tag>
      ),
    },
    ...(verification.badge
      ? [
          {
            key: "badge",
            label: "Huy hiệu",
            children: <Tag color="success">Huy hiệu đã xác minh</Tag>,
          },
        ]
      : []),
  ];

  return (
    <AccountShell
      current="verification"
      title="Xác minh tài khoản"
      description="Gửi giấy tờ để xác minh danh tính (KYC) và nhận huy hiệu tài khoản."
    >
      <Card>
        <CardHeader>
          <h2 className="text-base font-semibold text-text-primary">
            Trạng thái hiện tại
          </h2>
        </CardHeader>
        <CardContent>
          <Descriptions items={items} column={1} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <h2 className="text-base font-semibold text-text-primary">
            Gửi hồ sơ xác minh
          </h2>
        </CardHeader>
        <CardContent>
          <SubmitKycForm />
        </CardContent>
      </Card>
    </AccountShell>
  );
}
