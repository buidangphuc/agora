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
        className="flex items-center rounded-lg bg-white p-1 shadow-sm border border-transparent focus-within:border-primary-300 focus-within:ring-2 focus-within:ring-primary-100 transition duration-150"
      >
        <div className="pl-3 pr-2 text-gray-400">
          <svg
            className="w-4 h-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <title>Tìm kiếm</title>
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
            />
          </svg>
        </div>
        <input
          value={query}
          onChange={(e) => handleChange(e.target.value)}
          onFocus={() => setIsOpen(true)}
          onBlur={() => setTimeout(() => setIsOpen(false), 150)}
          placeholder="Tìm kiếm sản phẩm, thương hiệu, hoặc deal hời hôm nay..."
          className="flex-1 bg-transparent py-1.5 text-xs sm:text-sm text-gray-900 outline-none placeholder:text-gray-400"
        />
        <button
          type="submit"
          className="flex items-center justify-center rounded-md bg-primary-500 px-5 py-2 text-xs font-semibold text-white shadow-xs hover:bg-primary-600 active:bg-primary-700 transition cursor-pointer"
        >
          Tìm Kiếm
        </button>
      </form>

      {/* Suggested Keywords Strip */}
      <div className="mt-1.5 flex flex-wrap gap-2 text-xs text-white/90">
        {TRENDING_KEYWORDS.map((kw) => (
          <Link
            key={kw}
            href={`/search?q=${encodeURIComponent(kw)}`}
            className="hover:text-yellow-200 transition font-medium"
          >
            {kw}
          </Link>
        ))}
      </div>

      {/* Autocomplete Dropdown */}
      {isOpen && suggestions.length > 0 && (
        <ul className="absolute z-40 mt-1.5 w-full overflow-hidden rounded-xl border border-gray-100 bg-white shadow-xl py-1">
          {suggestions.map((s) => (
            <li key={s}>
              <button
                type="button"
                onMouseDown={() => handleSearch(s)}
                className="flex w-full items-center justify-between px-4 py-2.5 text-left text-xs text-gray-800 hover:bg-primary-50/60 hover:text-primary-600 transition"
              >
                <div className="flex items-center gap-2.5">
                  <span className="text-gray-400">🔍</span>
                  <span className="font-medium">{s}</span>
                </div>
                <span className="text-xs text-primary-500 font-semibold">
                  Tìm kiếm →
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
