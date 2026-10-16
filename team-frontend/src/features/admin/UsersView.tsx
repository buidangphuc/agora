"use client";

import Link from "next/link";
import { useState } from "react";

import type { AdminUserView, UserStatusName } from "@/lib/gateway/users";

import { setUserStatusAction } from "./userActions";
import type { UserFilters } from "./userFilters";

const STATUS_LABEL: Record<UserStatusName, string> = {
  active: "Hoạt động",
  restricted: "Hạn chế",
  suspended: "Tạm khóa",
  banned: "Cấm",
};
const STATUS_ORDER: UserStatusName[] = [
  "active",
  "restricted",
  "suspended",
  "banned",
];
const REASONS = [
  "policy_violation",
  "fraud",
  "counterfeit",
  "abuse",
  "scam",
  "spam",
  "other",
];

const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
const pad = (n: number) => String(n).padStart(2, "0");

// YYYY-MM-DD HH:mm in UTC+07:00 (fixed, so server and browser agree).
function formatVn(iso: string): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const d = new Date(t + VN_OFFSET_MS);
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}`;
}

function nextHref(filters: UserFilters, cursor: string): string {
  const q = new URLSearchParams();
  if (filters.query) q.set("query", filters.query);
  if (filters.status) q.set("status", filters.status);
  q.set("cursor", cursor);
  return `/admin/users?${q.toString()}`;
}

const inputClass =
  "w-full rounded-md border border-border-strong bg-surface-card px-3 py-1.5 text-sm text-text-primary";

const HEADERS = [
  "Tên đăng nhập",
  "ID",
  "Vai trò",
  "Trạng thái",
  "Lý do",
  "Hết hạn lúc",
  "Thao tác",
];

function RowForm({
  userId,
  onResult,
}: {
  userId: string;
  onResult: (r: { ok: boolean; text: string }) => void;
}) {
  const id = (n: string) => `users-${userId}-${n}`;
  return (
    <form
      data-testid="users-status-form"
      className="flex flex-col gap-2"
      action={async (fd) => {
        const res = await setUserStatusAction(fd);
        onResult(
          res.ok
            ? { ok: true, text: res.message }
            : { ok: false, text: res.error },
        );
      }}
    >
      <input type="hidden" name="userId" value={userId} />
      <label htmlFor={id("status")} className="text-xs text-text-secondary">
        Trạng thái mới
      </label>
      <select id={id("status")} name="status" className={inputClass}>
        {STATUS_ORDER.map((s) => (
          <option key={s} value={s}>
            {STATUS_LABEL[s]}
          </option>
        ))}
      </select>
      <label htmlFor={id("reason")} className="text-xs text-text-secondary">
        Mã lý do
      </label>
      <select id={id("reason")} name="reason" className={inputClass}>
        <option value="">Chọn lý do</option>
        {REASONS.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
      <label htmlFor={id("until")} className="text-xs text-text-secondary">
        Tạm khóa đến
      </label>
      <input
        id={id("until")}
        name="until"
        type="datetime-local"
        className={inputClass}
      />
      <label htmlFor={id("note")} className="text-xs text-text-secondary">
        Ghi chú (tối đa 500 ký tự)
      </label>
      <textarea
        id={id("note")}
        name="note"
        maxLength={500}
        className={inputClass}
      />
      <button
        type="submit"
        className="rounded-md bg-action-primary px-3 py-1.5 text-sm text-text-inverse hover:bg-action-primary-hover"
      >
        Cập nhật trạng thái
      </button>
    </form>
  );
}

export function UsersView({
  users,
  nextCursor,
  filters,
  selfId,
  error,
}: {
  users: AdminUserView[];
  nextCursor: string;
  filters: UserFilters;
  selfId: string;
  error?: "unavailable";
}) {
  const [flash, setFlash] = useState<{ ok: boolean; text: string } | null>(
    null,
  );
  return (
    <div className="mx-auto max-w-6xl p-6">
      <h1 className="text-2xl font-bold text-text-primary">Người dùng</h1>

      <form
        data-testid="users-search"
        method="get"
        action="/admin/users"
        className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-3"
      >
        <div className="flex flex-col gap-1">
          <label htmlFor="users-query" className="text-xs text-text-secondary">
            Tên đăng nhập hoặc ID
          </label>
          <input
            id="users-query"
            name="query"
            type="text"
            defaultValue={filters.query}
            className={inputClass}
          />
        </div>
        <div className="flex flex-col gap-1">
          <label
            htmlFor="users-filter-status"
            className="text-xs text-text-secondary"
          >
            Trạng thái
          </label>
          <select
            id="users-filter-status"
            name="status"
            defaultValue={filters.status ?? ""}
            className={inputClass}
          >
            <option value="">Tất cả</option>
            {STATUS_ORDER.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABEL[s]}
              </option>
            ))}
          </select>
        </div>
        <div className="flex items-end">
          <button
            type="submit"
            className="rounded-md bg-action-primary px-4 py-2 text-sm text-text-inverse hover:bg-action-primary-hover"
          >
            Tìm
          </button>
        </div>
      </form>

      <div data-testid="users-flash" aria-live="polite" className="mt-3">
        {flash ? (
          <p
            role={flash.ok ? "status" : "alert"}
            className="rounded-md border border-border-strong bg-surface-muted p-3 text-sm text-text-primary"
          >
            {flash.text}
          </p>
        ) : null}
      </div>

      {error ? (
        <div
          role="alert"
          className="mt-4 rounded-md border border-border-strong bg-surface-muted p-3 text-sm text-text-primary"
        >
          Không tải được danh sách người dùng. Vui lòng thử lại sau.
        </div>
      ) : users.length === 0 ? (
        <p className="mt-4 text-sm text-text-secondary">
          Không tìm thấy người dùng nào
        </p>
      ) : (
        <table className="mt-4 w-full text-left text-sm">
          <thead>
            <tr>
              {HEADERS.map((h) => (
                <th key={h} scope="col" className="p-2">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id} data-testid="users-row" className="align-top">
                <td className="p-2">{u.username}</td>
                <td className="p-2">{u.id}</td>
                <td className="p-2">{u.roles.join(", ")}</td>
                <td className="p-2">{STATUS_LABEL[u.status]}</td>
                <td className="p-2">{u.statusReason}</td>
                <td className="p-2">{formatVn(u.statusUntil)}</td>
                <td className="p-2">
                  {u.id === selfId || u.roles.includes("admin") ? null : (
                    <RowForm userId={u.id} onResult={setFlash} />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {nextCursor ? (
        <Link
          href={nextHref(filters, nextCursor)}
          className="mt-4 inline-block text-sm text-text-secondary"
        >
          Trang sau
        </Link>
      ) : null}
    </div>
  );
}
