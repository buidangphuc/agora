import { Link } from "@/components/ui/Link";
import type { AuditParams } from "./auditFilters";

export interface AuditEventRow {
  id: string;
  actorId: string;
  action: string;
  targetType: string;
  targetId: string;
  createdAt: string;
  metadata: Record<string, string>;
}

export type AuditViewError = "invalid_range" | "unavailable" | "invalid_date";

const ERROR_TEXT: Record<AuditViewError, string> = {
  invalid_range:
    "Khoảng thời gian không hợp lệ: ngày kết thúc phải sau hoặc trùng ngày bắt đầu.",
  invalid_date: "Ngày không hợp lệ. Vui lòng chọn lại.",
  unavailable: "Không tải được nhật ký kiểm toán. Vui lòng thử lại sau.",
};

const VN_OFFSET_MS = 7 * 60 * 60 * 1000;
const pad = (n: number) => String(n).padStart(2, "0");

// YYYY-MM-DD HH:mm:ss in UTC+07:00 (fixed, so server and browser agree).
function formatVn(iso: string): string {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return iso;
  const d = new Date(t + VN_OFFSET_MS);
  return `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())}:${pad(d.getUTCSeconds())}`;
}

function formatMetadata(m: Record<string, string>): string {
  return Object.keys(m)
    .sort()
    .map((k) => `${k}=${m[k]}`)
    .join(", ");
}

const KEYS = [
  "target_type",
  "target_id",
  "actor_id",
  "action",
  "from",
  "to",
] as const;

function nextHref(params: AuditParams, cursor: string): string {
  const q = new URLSearchParams();
  for (const k of KEYS) {
    const v = params[k];
    if (v) q.set(k, v);
  }
  q.set("cursor", cursor);
  return `/admin/audit?${q.toString()}`;
}

const inputClass =
  "w-full rounded-md border border-border-strong bg-surface-card px-3 py-1.5 text-sm text-text-primary";

function Field({
  label,
  name,
  type = "text",
  value,
}: {
  label: string;
  name: (typeof KEYS)[number];
  type?: string;
  value?: string;
}) {
  const id = `audit-${name}`;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs text-text-secondary">
        {label}
      </label>
      <input
        id={id}
        name={name}
        type={type}
        defaultValue={value ?? ""}
        className={inputClass}
      />
    </div>
  );
}

export function AuditLogView({
  events,
  nextCursor,
  params,
  error,
}: {
  events: AuditEventRow[];
  nextCursor: string;
  params: AuditParams;
  error?: AuditViewError;
}) {
  return (
    <div className="mx-auto max-w-6xl p-6">
      <h1 className="text-2xl font-bold text-text-primary">
        Nhật ký kiểm toán
      </h1>
      <p className="mt-1 text-sm text-text-secondary">
        Tra cứu các thao tác quản trị: ai đã làm gì, lên đối tượng nào, vào lúc
        nào.
      </p>

      <form
        method="get"
        action="/admin/audit"
        className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-3"
      >
        <Field
          label="Loại đối tượng"
          name="target_type"
          value={params.target_type}
        />
        <Field label="Mã đối tượng" name="target_id" value={params.target_id} />
        <Field
          label="Người thực hiện (ID)"
          name="actor_id"
          value={params.actor_id}
        />
        <Field label="Hành động" name="action" value={params.action} />
        <Field label="Từ ngày" name="from" type="date" value={params.from} />
        <Field label="Đến ngày" name="to" type="date" value={params.to} />
        <div className="flex items-center gap-3">
          <button
            type="submit"
            className="rounded-md bg-action-primary px-4 py-2 text-sm text-text-inverse hover:bg-action-primary-hover"
          >
            Lọc
          </button>
          <span className="text-sm text-text-secondary">
            <Link href="/admin/audit" variant="subtle">
              Xóa bộ lọc
            </Link>
          </span>
        </div>
      </form>

      {error ? (
        <div
          role="alert"
          className="mt-4 rounded-md border border-border-strong bg-surface-muted p-3 text-sm text-text-primary"
        >
          <p>{ERROR_TEXT[error]}</p>
        </div>
      ) : events.length === 0 ? (
        <p className="mt-6 text-sm text-text-secondary">Không có sự kiện nào</p>
      ) : (
        <>
          <table className="mt-4 w-full text-left text-sm">
            <thead>
              <tr>
                <th>Thời gian</th>
                <th>Người thực hiện</th>
                <th>Hành động</th>
                <th>Đối tượng</th>
                <th>Chi tiết</th>
              </tr>
            </thead>
            <tbody>
              {events.map((e) => (
                <tr key={e.id}>
                  <td>{formatVn(e.createdAt)}</td>
                  <td>{e.actorId}</td>
                  <td>{e.action}</td>
                  <td>{`${e.targetType}/${e.targetId}`}</td>
                  <td>{formatMetadata(e.metadata)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {nextCursor ? (
            <p className="mt-4">
              <span className="text-sm">
                <Link href={nextHref(params, nextCursor)}>Trang sau</Link>
              </span>
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
