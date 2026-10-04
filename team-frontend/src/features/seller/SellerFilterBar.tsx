"use client";

import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Input } from "@/components/ui/Input";
import { Select, type SelectOption } from "@/components/ui/Select";
import { buildListHref } from "./listParams";

export const FILTER_DEBOUNCE_MS = 300;

/**
 * The only client island of a seller list: a debounced search box (and an
 * optional status select) that rewrites the URL with `router.replace`.
 * Changing a filter drops `page`, so the list restarts at page 1. The table
 * and pagination stay server-rendered.
 */
export function SellerFilterBar({
  basePath,
  q,
  status,
  statusOptions,
  fixed = {},
  searchLabel = "Tìm kiếm",
  placeholder = "Tìm theo tên",
  hint,
}: {
  basePath: string;
  q: string;
  status?: string;
  statusOptions?: SelectOption[];
  /** Params kept as they are (for example the active order tab). */
  fixed?: Record<string, string>;
  searchLabel?: string;
  placeholder?: string;
  hint?: string;
}) {
  const router = useRouter();
  const [text, setText] = useState(q);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  function go(nextQ: string, nextStatus: string | undefined) {
    router.replace(
      buildListHref(basePath, {
        ...fixed,
        q: nextQ.trim(),
        ...(nextStatus === undefined ? {} : { status: nextStatus }),
      }),
    );
  }

  function onSearch(value: string) {
    setText(value);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => go(value, status), FILTER_DEBOUNCE_MS);
  }

  return (
    <div className="space-y-1.5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="min-w-0 flex-1">
          <Input
            type="search"
            label={searchLabel}
            placeholder={placeholder}
            value={text}
            onChange={(e) => onSearch(e.target.value)}
          />
        </div>
        {statusOptions && (
          <div className="w-full space-y-1.5 sm:w-48">
            <label
              htmlFor="seller-status-filter"
              className="block text-xs font-medium text-text-primary"
            >
              Trạng thái
            </label>
            <Select
              id="seller-status-filter"
              options={statusOptions}
              value={status ?? "all"}
              onChange={(e) => {
                if (timer.current) clearTimeout(timer.current);
                go(text, e.target.value);
              }}
            />
          </div>
        )}
      </div>
      {hint && <p className="text-xs text-text-secondary">{hint}</p>}
    </div>
  );
}
