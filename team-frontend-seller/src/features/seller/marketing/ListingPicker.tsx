"use client";

import { useEffect, useRef, useState } from "react";

import { Input } from "@/components/ui/Input";
import { searchMyListingsAction } from "../actions";

export interface PickedListing {
  id: string;
  title: string;
}

const DEBOUNCE_MS = 300;

export function ListingPicker({
  onSelect,
}: { onSelect: (l: PickedListing) => void }) {
  const [query, setQuery] = useState("");
  const [options, setOptions] = useState<PickedListing[]>([]);
  const [error, setError] = useState("");
  const touched = useRef(false);

  useEffect(() => {
    if (!touched.current) return;
    let stale = false;
    const t = setTimeout(async () => {
      const res = await searchMyListingsAction(query);
      if (stale) return;
      if (res.ok && res.data) {
        setError("");
        setOptions(
          res.data.page.items.map((l) => ({ id: l.id, title: l.title })),
        );
      } else if (!res.ok) {
        setError(res.error);
      }
    }, DEBOUNCE_MS);
    return () => {
      stale = true;
      clearTimeout(t);
    };
  }, [query]);

  return (
    <div className="space-y-2">
      <Input
        label="Tìm sản phẩm"
        value={query}
        onChange={(e) => {
          touched.current = true;
          setQuery(e.target.value);
        }}
      />
      {error && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
      {options.length > 0 && (
        <ul className="space-y-1">
          {options.map((o) => (
            <li key={o.id}>
              <button
                type="button"
                className="w-full rounded px-2 py-1 text-left hover:bg-surface-muted"
                onClick={() => {
                  onSelect(o);
                  setOptions([]);
                }}
              >
                {o.title}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
