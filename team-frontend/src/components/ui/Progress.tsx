import React from "react";

export interface ProgressProps {
  /** 0-100 (clamped). */
  percent: number;
  status?: "normal" | "success" | "danger";
  /** Show the percentage text beside the bar. */
  showInfo?: boolean;
  size?: "sm" | "md";
  /** Accessible name. */
  label?: string;
  className?: string;
}

// Tier 3: component tokens.
const fill = {
  normal: "bg-action-primary",
  success: "bg-success",
  danger: "bg-danger",
};

/** Ant Design `Progress` (line). Server-compatible; exposes aria-valuenow. */
export function Progress({
  percent,
  status = "normal",
  showInfo = true,
  size = "md",
  label = "Tiến độ",
  className = "",
}: ProgressProps) {
  const value = Math.min(100, Math.max(0, Math.round(percent)));
  return (
    <div className={`flex items-center gap-3 ${className}`}>
      {/* Native <progress> carries the semantics; the bar below is visual only. */}
      <progress
        className="sr-only"
        aria-label={label}
        aria-valuenow={value}
        aria-valuemin={0}
        aria-valuemax={100}
        value={value}
        max={100}
      />
      <div
        aria-hidden="true"
        className={`w-full overflow-hidden rounded-full bg-border-subtle ${
          size === "sm" ? "h-1.5" : "h-2.5"
        }`}
      >
        <div
          className={`h-full rounded-full transition-all duration-200 ${fill[status]}`}
          style={{ width: `${value}%` }}
        />
      </div>
      {showInfo && (
        <span className="w-10 shrink-0 text-right text-xs text-text-secondary">
          {value}%
        </span>
      )}
    </div>
  );
}
