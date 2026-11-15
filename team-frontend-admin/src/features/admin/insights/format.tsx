import type { ReactNode } from "react";

export const NO_DATA = "—";

export function pct(v: number | null | undefined): string {
  if (typeof v !== "number" || !Number.isFinite(v)) return NO_DATA;
  return `${(v * 100).toFixed(1)}%`;
}

export function num(v: number | bigint | null | undefined): string {
  if (v === null || v === undefined) return NO_DATA;
  return v.toLocaleString("en-US");
}

export const UNAVAILABLE_TEXT = "Tạm thời không có số liệu";

export function Note({ children }: { children: ReactNode }) {
  return <p>{children}</p>;
}
