import { redirect } from "next/navigation";

import { SubmitKycForm } from "@/features/account/verification/SubmitKycForm";
import { getPrincipal } from "@/lib/gateway/session";
import {
  VerificationStatus,
  getVerificationStatus,
} from "@/lib/gateway/verification";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Xác minh tài khoản | Marketplace",
};

function badgeClass(status: VerificationStatus): string {
  switch (status) {
    case VerificationStatus.VERIFIED:
      return "bg-green-100 text-green-700";
    case VerificationStatus.PENDING:
      return "bg-amber-100 text-amber-700";
    case VerificationStatus.REJECTED:
      return "bg-red-100 text-red-700";
    default:
      return "bg-gray-100 text-gray-500";
  }
}

export default async function VerificationPage() {
  if (!getPrincipal()) redirect("/login");

  const verification = await getVerificationStatus();

  return (
    <section className="space-y-6 py-2">
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">
          ✅ Xác minh tài khoản
        </h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Gửi giấy tờ để xác minh danh tính (KYC) và nhận huy hiệu tài khoản.
        </p>
      </div>

      {/* ── Current status ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Trạng thái hiện tại
        </h2>
        <div className="flex items-center gap-3">
          <span
            className={`rounded-full px-3 py-1 text-xs font-semibold ${badgeClass(
              verification.status,
            )}`}
          >
            {verification.statusText}
          </span>
          {verification.badge && (
            <span className="flex items-center gap-1 text-xs font-medium text-blue-600">
              ✔ Huy hiệu đã xác minh
            </span>
          )}
        </div>
      </div>

      {/* ── Submit KYC ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Gửi hồ sơ xác minh
        </h2>
        <SubmitKycForm />
      </div>
    </section>
  );
}
