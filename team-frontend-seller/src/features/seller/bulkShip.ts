import { Code, ConnectError } from "@connectrpc/connect";

export const BULK_SHIP_MAX = 50;
export const BULK_SHIP_CONCURRENCY = 4;
export const BULK_SHIP_DEADLINE_MS = 15_000;
export const NOT_ATTEMPTED_REASON = "Chưa xử lý, hãy thử lại";

const MAX_ERROR = "Chọn tối đa 50 đơn";
const ID_PATTERN = /^[A-Za-z0-9_-]+$/;

export type ParseIdsResult =
  | { ok: true; ids: string[] }
  | { ok: false; error: string };

export function parseIds(raw: string | null | undefined): ParseIdsResult {
  if (!raw) return { ok: false, error: "Chưa chọn đơn nào" };
  const seen = new Set<string>();
  for (const part of raw.split(",")) {
    const id = part.trim();
    if (!id) continue;
    if (!ID_PATTERN.test(id)) {
      return { ok: false, error: "Danh sách đơn không hợp lệ" };
    }
    seen.add(id);
  }
  if (seen.size === 0) return { ok: false, error: "Chưa chọn đơn nào" };
  if (seen.size > BULK_SHIP_MAX) return { ok: false, error: MAX_ERROR };
  return { ok: true, ids: [...seen] };
}

export function toggleSelection(
  selected: string[],
  id: string,
): { selected: string[]; error?: string } {
  if (selected.includes(id)) {
    return { selected: selected.filter((s) => s !== id) };
  }
  if (selected.length >= BULK_SHIP_MAX) {
    return { selected, error: MAX_ERROR };
  }
  return { selected: [...selected, id] };
}

export function reasonForError(err: unknown): string {
  if (err instanceof ConnectError) {
    switch (err.code) {
      case Code.PermissionDenied:
        return "Không có quyền với đơn này";
      case Code.NotFound:
        return "Không tìm thấy đơn";
      case Code.FailedPrecondition:
        return "Trạng thái đơn không cho phép gửi (có thể đã huỷ hoặc đã gửi)";
      case Code.Unavailable:
      case Code.DeadlineExceeded:
        return "Dịch vụ đơn hàng đang bận";
    }
  }
  return "Cập nhật thất bại";
}

export type PoolResult<R> = { done: true; value: R } | { done: false };

export async function runPool<T, R>(
  items: readonly T[],
  fn: (item: T) => Promise<R>,
  opts: {
    concurrency?: number;
    deadlineMs?: number;
    now?: () => number;
  } = {},
): Promise<PoolResult<R>[]> {
  const limit = opts.concurrency ?? BULK_SHIP_CONCURRENCY;
  const deadlineMs = opts.deadlineMs ?? BULK_SHIP_DEADLINE_MS;
  const now = opts.now ?? Date.now;
  const start = now();
  const out: PoolResult<R>[] = items.map(() => ({ done: false }));
  let next = 0;

  async function worker(): Promise<void> {
    while (next < items.length) {
      const i = next++;
      if (i >= limit && now() - start >= deadlineMs) return;
      out[i] = { done: true, value: await fn(items[i]) };
    }
  }

  await Promise.all(
    Array.from({ length: Math.min(limit, items.length) }, worker),
  );
  return out;
}
