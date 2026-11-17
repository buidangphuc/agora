"use client";

import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { searchMyListingsAction } from "./actions";

export interface PickedListing {
  id: string;
  title: string;
}

const DEBOUNCE_MS = 300;

export function ListingPicker({
  selectedIds,
  onToggle,
}: {
  selectedIds: string[];
  onToggle: (l: PickedListing) => void;
}) {
  const [query, setQuery] = useState("");
  const [options, setOptions] = useState<PickedListing[]>([]);
  const [failed, setFailed] = useState(false);
  const [tick, setTick] = useState(0);
  const immediate = useRef(true);

  // biome-ignore lint/correctness/useExhaustiveDependencies: tick re-runs the search on retry
  useEffect(() => {
    let stale = false;
    const t = setTimeout(
      async () => {
        const res = await searchMyListingsAction(query);
        if (stale) return;
        if (res.ok && res.data) {
          setFailed(false);
          setOptions(
            res.data.page.items.map((l) => ({ id: l.id, title: l.title })),
          );
        } else {
          setFailed(true);
        }
      },
      immediate.current ? 0 : DEBOUNCE_MS,
    );
    return () => {
      stale = true;
      clearTimeout(t);
    };
  }, [query, tick]);

  return (
    <div className="space-y-2" data-testid="listing-picker">
      <Input
        label="Tìm sản phẩm"
        value={query}
        onChange={(e) => {
          immediate.current = false;
          setQuery(e.target.value);
        }}
      />
      {failed ? (
        <div role="alert" className="space-y-2">
          <p className="text-sm text-danger">Không tải được sản phẩm</p>
          <Button
            type="button"
            onClick={() => {
              immediate.current = true;
              setTick((n) => n + 1);
            }}
          >
            Thử lại
          </Button>
        </div>
      ) : options.length === 0 ? (
        <p className="text-sm text-text-secondary">Không tìm thấy sản phẩm</p>
      ) : (
        <ul className="max-h-56 space-y-2 overflow-y-auto rounded-xl border border-border-subtle p-3">
          {options.map((o) => (
            <li key={o.id} data-testid="listing-picker-option">
              <Checkbox
                label={o.title}
                checked={selectedIds.includes(o.id)}
                onCheckedChange={() => onToggle(o)}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
