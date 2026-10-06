"use client";

import { useRouter } from "next/navigation";
import { useEffect, useId, useRef, useState } from "react";

import { Button } from "@/components/ui/Button";

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

const SUGGEST_DEBOUNCE_MS = 200;

// WAI-ARIA combobox pattern: a text input controlling a listbox of options.
// A native <select>/<option> cannot sit beside a text input, so the roles are
// declared here (the lint rule that prefers native elements does not apply).
const LISTBOX = { role: "listbox", tabIndex: -1 } as const;
const OPTION = { role: "option", tabIndex: -1 } as const;

/**
 * Header search combobox. Submits to /search?q=<term>; suggestions come from
 * /api/suggest (debounced 200ms) and a failed request just hides them. An
 * empty input shows trending keywords when focused. Keyboard: ArrowUp/Down
 * move through the options, Enter submits the active one (or the typed text),
 * Escape closes. An empty submit does nothing.
 */
export function SearchBar() {
  const router = useRouter();
  const listId = useId();
  const [query, setQuery] = useState("");
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [isOpen, setIsOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const latest = useRef(0);

  useEffect(() => () => clearTimeout(timer.current), []);

  const typed = query.trim();
  const options = typed ? suggestions : TRENDING_KEYWORDS;
  const showList = isOpen && options.length > 0;
  const optionId = (i: number) => `${listId}-opt-${i}`;

  function submit(term: string) {
    const q = term.trim();
    if (!q) return;
    setIsOpen(false);
    setActive(-1);
    router.push(`/search?q=${encodeURIComponent(q)}`);
  }

  function handleChange(value: string) {
    setQuery(value);
    setActive(-1);
    setIsOpen(true);
    clearTimeout(timer.current);
    const term = value.trim();
    latest.current += 1;
    const ticket = latest.current;
    if (!term) {
      setSuggestions([]);
      return;
    }
    timer.current = setTimeout(async () => {
      try {
        const res = await fetch(`/api/suggest?q=${encodeURIComponent(term)}`);
        if (ticket !== latest.current) return;
        if (!res.ok) {
          setSuggestions([]);
          return;
        }
        const data = (await res.json()) as { suggestions?: string[] };
        if (ticket === latest.current) setSuggestions(data.suggestions ?? []);
      } catch {
        // Suggestions are best-effort: hide them, keep the input working.
        if (ticket === latest.current) setSuggestions([]);
      }
    }, SUGGEST_DEBOUNCE_MS);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setIsOpen(true);
      if (options.length > 0) setActive((i) => (i + 1) % options.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (options.length > 0) {
        setActive((i) => (i <= 0 ? options.length - 1 : i - 1));
      }
    } else if (e.key === "Escape") {
      setIsOpen(false);
      setActive(-1);
    } else if (e.key === "Enter" && showList && active >= 0) {
      e.preventDefault();
      submit(options[active] ?? query);
    }
  }

  return (
    <div
      className="relative w-full"
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) setIsOpen(false);
      }}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault();
          submit(query);
        }}
        className="flex items-center gap-1 rounded-lg border border-transparent bg-surface-card p-1 shadow-sm transition duration-150 focus-within:border-primary-300 focus-within:ring-2 focus-within:ring-focus-ring"
      >
        <svg
          className="ml-3 mr-1 h-4 w-4 shrink-0 text-text-disabled"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          aria-hidden="true"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
          />
        </svg>
        <input
          role="combobox"
          aria-label="Tìm kiếm sản phẩm"
          aria-expanded={showList}
          aria-controls={listId}
          aria-autocomplete="list"
          aria-activedescendant={
            showList && active >= 0 ? optionId(active) : undefined
          }
          autoComplete="off"
          value={query}
          onChange={(e) => handleChange(e.target.value)}
          onFocus={() => setIsOpen(true)}
          onKeyDown={handleKeyDown}
          placeholder="Tìm kiếm sản phẩm, thương hiệu, hoặc deal hời hôm nay..."
          className="min-h-9 min-w-0 flex-1 bg-transparent px-1 text-sm text-text-primary outline-none placeholder:text-text-disabled"
        />
        <Button type="submit" size="md">
          Tìm Kiếm
        </Button>
      </form>

      {showList && (
        <div className="absolute z-40 mt-1.5 w-full overflow-hidden rounded-xl border border-border-subtle bg-surface-card py-1 shadow-preline-hover">
          {!typed && (
            <p className="px-4 pb-1 pt-2 text-xs font-medium text-text-secondary">
              Tìm kiếm phổ biến
            </p>
          )}
          <div id={listId} aria-label="Gợi ý tìm kiếm" {...LISTBOX}>
            {options.map((s, i) => (
              <div
                key={s}
                id={optionId(i)}
                aria-selected={i === active}
                {...OPTION}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => submit(s)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") submit(s);
                }}
                className={`flex min-h-11 cursor-pointer items-center justify-between gap-3 px-4 text-left text-sm text-text-primary transition duration-150 ${
                  i === active ? "bg-primary-50" : "hover:bg-surface-muted"
                }`}
              >
                <span className="truncate font-medium">{s}</span>
                <span className="shrink-0 text-xs font-semibold text-action-primary">
                  Tìm kiếm
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
