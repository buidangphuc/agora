import { redirect } from "next/navigation";

import { formatPrice } from "@/components/ui/format";
import { GenerateReferralCodeButton } from "@/features/account/referral/GenerateReferralCodeButton";
import { RedeemReferralForm } from "@/features/account/referral/RedeemReferralForm";
import {
  getMyReferral,
  listReferralRewards,
} from "@/lib/gateway/referral";
import { getPrincipal } from "@/lib/gateway/session";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Mời bạn bè | Marketplace",
};

export default async function ReferralPage() {
  if (!getPrincipal()) redirect("/login");

  const [referral, rewards] = await Promise.all([
    getMyReferral(),
    listReferralRewards(),
  ]);

  return (
    <section className="space-y-6 py-2">
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">🎁 Mời bạn bè</h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Chia sẻ mã giới thiệu của bạn để nhận thưởng khi bạn bè tham gia.
        </p>
      </div>

      {/* ── My referral summary ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Mã giới thiệu của bạn
        </h2>
        {referral.code ? (
          <div className="flex flex-wrap items-center gap-3">
            <span className="rounded-lg border border-dashed border-brand bg-brand/5 px-4 py-2 font-mono text-lg font-bold tracking-wider text-brand">
              {referral.code}
            </span>
          </div>
        ) : (
          <div className="space-y-2">
            <p className="text-xs text-gray-400">
              Bạn chưa có mã giới thiệu. Tạo ngay để bắt đầu mời bạn bè.
            </p>
            <GenerateReferralCodeButton />
          </div>
        )}

        <div className="mt-4 grid grid-cols-2 gap-3">
          <div className="rounded-xl bg-gray-50 p-3">
            <p className="text-xs text-gray-500">Đã mời</p>
            <p className="text-xl font-bold text-gray-900">
              {referral.invitedCount}
            </p>
          </div>
          <div className="rounded-xl bg-gray-50 p-3">
            <p className="text-xs text-gray-500">Tổng thưởng</p>
            <p className="text-xl font-bold text-brand">
              {formatPrice(referral.rewardsTotal)}
            </p>
          </div>
        </div>
      </div>

      {/* ── Redeem a friend's code ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Nhập mã của bạn bè
        </h2>
        <RedeemReferralForm />
      </div>

      {/* ── Rewards ledger ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Lịch sử thưởng ({rewards.length})
        </h2>
        {rewards.length === 0 ? (
          <p className="text-xs text-gray-400">Chưa có phần thưởng nào.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {rewards.map((r) => (
              <li
                key={r.id}
                className="flex items-center justify-between gap-3 py-2.5 text-sm"
              >
                <div className="min-w-0">
                  <p className="font-medium text-gray-800">{r.reason || "—"}</p>
                  <p className="text-xs text-gray-400">{r.createdAt}</p>
                </div>
                <span className="shrink-0 font-semibold text-green-600">
                  +{formatPrice(r.amount)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
