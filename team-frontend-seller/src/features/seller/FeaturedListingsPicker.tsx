"use client";

import { useEffect, useRef, useState } from "react";

import { searchMyListingsAction } from "./actions";
import { FEATURED_MAX, addFeatured, moveFeatured } from "./shopBranding";

type Item = { id: string; title: string };

const DEBOUNCE_MS = 300;

export function FeaturedListingsPicker({
  value,
  titles,
  onChange,
}: {
  value: string[];
  titles: Record<string, string>;
  onChange: (ids: string[]) => void;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Item[]>([]);
  const [error, setError] = useState("");
  const [known, setKnown] = useState<Record<string, string>>({});
  const seq = useRef(0);

  async function search(q: string) {
    const mine = ++seq.current;
    const res = await searchMyListingsAction(q);
    if (mine !== seq.current) return;
    if (res.ok && res.data) {
      const items = res.data.page.items as Item[];
      setError("");
      setResults(items);
      setKnown((k) => ({
        ...k,
        ...Object.fromEntries(items.map((i) => [i.id, i.title])),
      }));
    } else {
      setError(res.ok ? "Không tải được danh sách sản phẩm." : res.error);
    }
  }

  useEffect(() => {
    if (query.trim() === "") return;
    const t = setTimeout(() => void search(query), DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [query]);

  const full = value.length >= FEATURED_MAX;

  function add(id: string) {
    const r = addFeatured(value, id);
    if (r.ok) onChange(r.ids);
  }

  return (
    <div className="space-y-3">
      <label className="block text-sm font-medium text-text-primary">
        Tìm sản phẩm
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="mt-1 block w-full rounded border px-3 py-2 text-sm"
        />
      </label>
      {full && <p className="text-sm">Tối đa 6 sản phẩm nổi bật.</p>}
      {error && (
        <div role="alert" className="text-sm text-action-danger">
          <p>{error}</p>
          <button
            type="button"
            className="underline"
            onClick={() => void search(query)}
          >
            Thử lại
          </button>
        </div>
      )}
      {!error && results.length > 0 && (
        <ul className="space-y-1">
          {results.map((r) => (
            <li key={r.id} className="flex items-center justify-between">
              <span>{r.title}</span>
              <button
                type="button"
                disabled={full || value.includes(r.id)}
                onClick={() => add(r.id)}
              >
                {`Thêm ${r.title}`}
              </button>
            </li>
          ))}
        </ul>
      )}
      <ul className="space-y-1">
        {value.map((id, i) => {
          const title = titles[id] ?? known[id];
          return (
            <li key={id} className="flex items-center gap-2">
              <span className="flex-1">{title ?? "Không còn khả dụng"}</span>
              <button
                type="button"
                aria-label={`Lên ${title ?? id}`}
                disabled={i === 0}
                onClick={() => onChange(moveFeatured(value, i, -1))}
              >
                ↑
              </button>
              <button
                type="button"
                aria-label={`Xuống ${title ?? id}`}
                disabled={i === value.length - 1}
                onClick={() => onChange(moveFeatured(value, i, 1))}
              >
                ↓
              </button>
              <button
                type="button"
                aria-label={`Xóa ${title ?? id}`}
                onClick={() => onChange(value.filter((v) => v !== id))}
              >
                ×
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
