"use client";

import { usePathname, useRouter } from "next/navigation";

// StatusFilter navigates to the same page with a ?status= query so the RSC
// re-fetches. Used on Home and Search.
export function StatusFilter({ value }: { value: string }) {
  const router = useRouter();
  const pathname = usePathname();

  return (
    <select
      value={value}
      onChange={(e) => router.push(`${pathname}?status=${e.target.value}`)}
      className="rounded-md border px-3 py-1.5 text-sm outline-none focus:border-brand"
    >
      <option value="published">Đang bán</option>
      <option value="all">Tất cả</option>
      <option value="draft">Nháp</option>
    </select>
  );
}
