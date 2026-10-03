import React from "react";
import { RateInput } from "./RateInput";
import { RateStar, rateSizes } from "./rateStar";

export interface RateProps {
  /** Controlled value (read-only display accepts halves, e.g. 4.5). */
  value?: number;
  /** Uncontrolled initial value (input mode). */
  defaultValue?: number;
  count?: number;
  /** Static display: server-compatible, no client JavaScript. */
  readOnly?: boolean;
  onChange?: (value: number) => void;
  disabled?: boolean;
  /** Form field name for the input mode. */
  name?: string;
  size?: keyof typeof rateSizes;
  /** Accessible name of the input mode. */
  label?: string;
  className?: string;
}

/**
 * Ant Design `Rate`. `readOnly` renders a static, server-compatible
 * `role="img"` summary (halves allowed); otherwise an interactive radio group.
 */
export function Rate({
  value,
  defaultValue,
  count = 5,
  readOnly = false,
  onChange,
  disabled = false,
  name,
  size = "md",
  label = "Đánh giá",
  className = "",
}: RateProps) {
  if (!readOnly) {
    return (
      <RateInput
        value={value}
        defaultValue={defaultValue}
        count={count}
        onChange={onChange}
        disabled={disabled}
        name={name}
        size={size}
        label={label}
        className={className}
      />
    );
  }

  const shown = Math.min(count, Math.max(0, value ?? defaultValue ?? 0));
  return (
    <span
      role="img"
      aria-label={`${shown} trên ${count} sao`}
      className={`inline-flex gap-0.5 ${className}`}
    >
      {Array.from({ length: count }, (_, i) => i + 1).map((n) => (
        <RateStar
          key={n}
          size={size}
          fill={shown >= n ? 1 : shown >= n - 0.5 ? 0.5 : 0}
        />
      ))}
    </span>
  );
}
