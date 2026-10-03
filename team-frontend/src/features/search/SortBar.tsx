"use client";

import { useRouter, useSearchParams } from "next/navigation";

export function SortBar({
  currentSort = "relevance",
  totalResults = 0,
}: {
  currentSort?: string;
  totalResults?: number;
}) {
  const router = useRouter();
  const searchParams = useSearchParams();

  function handleSort(sortValue: string) {
    const params = new URLSearchParams(searchParams.toString());
    params.set("sort", sortValue);
    router.push(`/search?${params.toString()}`);
  }

  const sortButtons = [
    { id: "relevance", label: "Liên Quan" },
    { id: "newest", label: "Mới Nhất" },
    { id: "sales", label: "Bán Chạy" },
  ];

  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-xl bg-gray-100/80 p-3.5 text-xs border border-gray-200/60 shadow-2xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-gray-600 font-medium mr-1">Sắp xếp theo:</span>
        {sortButtons.map((btn) => {
          const isActive = currentSort === btn.id;
          return (
            <button
              key={btn.id}
              type="button"
              onClick={() => handleSort(btn.id)}
              className={`rounded-lg px-4 py-2 font-medium transition cursor-pointer ${
                isActive
                  ? "bg-primary-500 text-white shadow-xs font-semibold"
                  : "bg-white text-gray-700 hover:bg-gray-50 border border-gray-200/80"
              }`}
            >
              {btn.label}
            </button>
          );
        })}

        <select
          value={currentSort.startsWith("price_") ? currentSort : ""}
          onChange={(e) => {
            if (e.target.value) handleSort(e.target.value);
          }}
          className="rounded-lg border border-gray-200/80 bg-white px-3 py-2 text-xs text-gray-700 outline-none focus:border-primary-500 focus:ring-2 focus:ring-primary-100 cursor-pointer"
        >
          <option value="">Giá: Mặc định</option>
          <option value="price_asc">Giá: Thấp đến Cao</option>
          <option value="price_desc">Giá: Cao đến Thấp</option>
        </select>
      </div>

      <div className="text-xs text-gray-500">
        Tìm thấy{" "}
        <strong className="text-primary-600 font-bold">{totalResults}</strong>{" "}
        kết quả
      </div>
    </div>
  );
}
