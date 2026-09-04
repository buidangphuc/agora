import { redirect } from "next/navigation";

import { RevokeSessionButton } from "@/features/account/RevokeSessionButton";
import { getPrincipal } from "@/lib/gateway/session";
import { listLoginHistory, listSessions } from "@/lib/gateway/sessions";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Bảo mật tài khoản | Marketplace",
};

export default async function SecurityPage() {
  if (!getPrincipal()) redirect("/login");

  const [sessions, history] = await Promise.all([
    listSessions(),
    listLoginHistory(),
  ]);

  return (
    <section className="space-y-6 py-2">
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h1 className="text-lg font-bold text-gray-900">
          🔒 Bảo mật tài khoản
        </h1>
        <p className="mt-0.5 text-xs text-gray-500">
          Quản lý các phiên đăng nhập và xem lịch sử đăng nhập của bạn.
        </p>
      </div>

      {/* ── Active sessions ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Phiên đăng nhập ({sessions.length})
        </h2>
        {sessions.length === 0 ? (
          <p className="text-xs text-gray-400">
            Không có phiên nào đang hoạt động.
          </p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {sessions.map((s) => (
              <li
                key={s.id}
                className="flex items-center justify-between gap-3 py-3 text-sm"
              >
                <div className="min-w-0">
                  <p className="font-medium text-gray-800">
                    {s.device || "Thiết bị không xác định"}
                  </p>
                  <p className="text-xs text-gray-500">
                    IP: {s.ip || "—"} · Hoạt động: {s.lastSeen || s.createdAt}
                  </p>
                </div>
                {s.revoked ? (
                  <span className="text-xs text-gray-400">Đã thu hồi</span>
                ) : (
                  <RevokeSessionButton sessionId={s.id} />
                )}
              </li>
            ))}
          </ul>
        )}
      </div>

      {/* ── Login history ── */}
      <div className="rounded-2xl border border-gray-100 bg-white p-5 shadow-2xs">
        <h2 className="mb-3 text-sm font-bold text-gray-800">
          Lịch sử đăng nhập ({history.length})
        </h2>
        {history.length === 0 ? (
          <p className="text-xs text-gray-400">Chưa có lịch sử đăng nhập.</p>
        ) : (
          <ul className="divide-y divide-gray-100">
            {history.map((e) => (
              <li
                key={e.id}
                className="flex items-center justify-between gap-3 py-2.5 text-xs"
              >
                <div className="min-w-0">
                  <p className="text-gray-700">{e.userAgent || "—"}</p>
                  <p className="text-gray-400">
                    IP: {e.ip || "—"} · {e.createdAt}
                  </p>
                </div>
                <span
                  className={`shrink-0 rounded px-2 py-0.5 font-semibold ${
                    e.success
                      ? "bg-green-100 text-green-700"
                      : "bg-red-100 text-red-700"
                  }`}
                >
                  {e.success ? "Thành công" : "Thất bại"}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
