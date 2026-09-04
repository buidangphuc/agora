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

  return (
    <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 rounded-xs bg-[#ededed] p-3 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-gray-600 font-medium mr-1">Sắp xếp theo:</span>
        <button
          type="button"
          onClick={() => handleSort("relevance")}
          className={`rounded-xs px-4 py-1.5 font-medium transition ${
            currentSort === "relevance"
              ? "bg-brand text-white shadow-2xs"
              : "bg-white text-gray-800 hover:bg-gray-50"
          }`}
        >
          Liên Quan
        </button>

        <button
          type="button"
          onClick={() => handleSort("newest")}
          className={`rounded-xs px-4 py-1.5 font-medium transition ${
            currentSort === "newest"
              ? "bg-brand text-white shadow-2xs"
              : "bg-white text-gray-800 hover:bg-gray-50"
          }`}
        >
          Mới Nhất
        </button>

        <button
          type="button"
          onClick={() => handleSort("sales")}
          className={`rounded-xs px-4 py-1.5 font-medium transition ${
            currentSort === "sales"
              ? "bg-brand text-white shadow-2xs"
              : "bg-white text-gray-800 hover:bg-gray-50"
          }`}
        >
          Bán Chạy
        </button>

        <select
          value={currentSort.startsWith("price_") ? currentSort : ""}
          onChange={(e) => {
            if (e.target.value) handleSort(e.target.value);
          }}
          className="rounded-xs border border-gray-300 bg-white px-3 py-1.5 text-xs text-gray-700 outline-none focus:border-brand"
        >
          <option value="">Giá: Mặc định</option>
          <option value="price_asc">Giá: Thấp đến Cao</option>
          <option value="price_desc">Giá: Cao đến Thấp</option>
        </select>
      </div>

      <div className="text-xs text-gray-500">
        Tìm thấy{" "}
        <strong className="text-brand font-bold">{totalResults}</strong> kết quả
      </div>
    </div>
  );
}
