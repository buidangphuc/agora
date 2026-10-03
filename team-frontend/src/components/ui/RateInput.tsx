"use client";

import React, { useId, useState } from "react";
import { RateStar, rateSizes } from "./rateStar";

export interface RateInputProps {
  value?: number;
  defaultValue?: number;
  count?: number;
  onChange?: (value: number) => void;
  disabled?: boolean;
  name?: string;
  size: keyof typeof rateSizes;
  className: string;
  label: string;
}

/** Interactive rating: native radios (arrow keys), hover preview. Client island. */
export function RateInput({
  value,
  defaultValue = 0,
  count = 5,
  onChange,
  disabled = false,
  name,
  size,
  className,
  label,
}: RateInputProps) {
  const autoName = useId();
  const [internal, setInternal] = useState(defaultValue);
  const [hover, setHover] = useState<number | null>(null);
  const current = value !== undefined ? value : internal;
  const shown = hover ?? current;

  const select = (n: number) => {
    if (value === undefined) setInternal(n);
    onChange?.(n);
  };

  return (
    <fieldset
      aria-label={label}
      disabled={disabled}
      aria-disabled={disabled ? "true" : undefined}
      className={`m-0 inline-flex min-w-0 gap-0.5 border-0 p-0 ${
        disabled ? "opacity-50 cursor-not-allowed" : ""
      } ${className}`}
      onMouseLeave={() => setHover(null)}
    >
      {Array.from({ length: count }, (_, i) => i + 1).map((n) => (
        <label
          key={n}
          className={`relative inline-flex ${disabled ? "" : "cursor-pointer"}`}
          onMouseEnter={() => !disabled && setHover(n)}
        >
          <input
            type="radio"
            name={name ?? autoName}
            value={n}
            checked={Math.round(current) === n}
            disabled={disabled}
            aria-label={`${n} sao`}
            onChange={() => select(n)}
            className="peer sr-only"
          />
          <RateStar
            fill={shown >= n ? 1 : 0}
            size={size}
            className="rounded-xs peer-focus-visible:ring-2 peer-focus-visible:ring-focus-ring peer-focus-visible:ring-offset-2"
          />
        </label>
      ))}
    </fieldset>
  );
}
