"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";

const TRENDING_KEYWORDS = [
  "iPhone 15 Pro Max",
  "MacBook Pro M3",
  "Tai Nghe Sony",
  "Áo Khoác Yody",
  "Nike Air Force 1",
  "Son Black Rouge",
  "Robot Hút Bụi",
  "Nồi Chiên Philips",
];

export function SearchBar() {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const debounceRef = useRef<NodeJS.Timeout>();

  function handleSearch(term: string) {
    const q = term.trim();
    if (!q) return;
    setIsOpen(false);
    router.push(`/search?q=${encodeURIComponent(q)}`);
  }

  function handleChange(val: string) {
    setQuery(val);
    if (!val.trim()) {
      setSuggestions([]);
      return;
    }

    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      try {
        const res = await fetch(
          `/api/suggest?q=${encodeURIComponent(val.trim())}`,
        );
        if (res.ok) {
          const data = await res.json();
          setSuggestions(data.suggestions || []);
        }
      } catch {
        setSuggestions([]);
      }
    }, 200);
  }

  return (
    <div className="relative w-full">
      {/* Search Input Box */}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          handleSearch(query);
        }}
        className="flex items-center rounded-sm bg-white p-1 shadow-sm"
      >
        <input
          value={query}
          onChange={(e) => handleChange(e.target.value)}
          onFocus={() => setIsOpen(true)}
          onBlur={() => setTimeout(() => setIsOpen(false), 150)}
          placeholder="Tìm kiếm sản phẩm, danh mục, thương hiệu..."
          className="flex-1 bg-transparent px-3 py-1.5 text-xs text-gray-900 outline-none placeholder-gray-400"
        />
        <button
          type="submit"
          className="flex items-center justify-center rounded-xs bg-brand px-6 py-2 text-xs font-bold text-white shadow-xs hover:bg-brand-dark transition"
        >
          🔍 Tìm Kiếm
        </button>
      </form>

      {/* Suggested Keywords Strip */}
      <div className="mt-1.5 flex flex-wrap gap-2.5 text-[11px] text-white/90">
        {TRENDING_KEYWORDS.map((kw) => (
          <Link
            key={kw}
            href={`/search?q=${encodeURIComponent(kw)}`}
            className="hover:text-yellow-200 transition"
          >
            {kw}
          </Link>
        ))}
      </div>

      {/* Autocomplete Dropdown */}
      {isOpen && suggestions.length > 0 && (
        <ul className="absolute z-40 mt-1 w-full overflow-hidden rounded-md border border-gray-100 bg-white shadow-xl">
          {suggestions.map((s) => (
            <li key={s}>
              <button
                type="button"
                onMouseDown={() => handleSearch(s)}
                className="flex w-full items-center justify-between px-4 py-2 text-left text-xs text-gray-800 hover:bg-orange-50 hover:text-brand"
              >
                <span>🔍 {s}</span>
                <span className="text-[10px] text-brand font-semibold">
                  Tìm kiếm sản phẩm
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
