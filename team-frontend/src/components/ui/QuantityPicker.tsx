"use client";

import React, { useId, useRef, useState } from "react";
import { focusRing } from "./focus";

export interface QuantityPickerProps {
  /** Controlled value. */
  value?: number;
  /** Uncontrolled initial value (defaults to `min`). */
  defaultValue?: number;
  min?: number;
  max?: number;
  step?: number;
  onChange?: (value: number) => void;
  disabled?: boolean;
  /** Accessible name of the field. */
  label?: string;
  size?: "sm" | "md";
  className?: string;
}

// Tier 3: component tokens.
const stepButton = `inline-flex items-center justify-center bg-surface-card text-text-primary transition duration-150 hover:bg-surface-page active:scale-95 ${focusRing} disabled:opacity-50 disabled:pointer-events-none disabled:cursor-not-allowed cursor-pointer`;
const sizes = {
  sm: { box: "h-8", button: "w-8", input: "w-10 text-xs" },
  md: { box: "h-10", button: "w-10", input: "w-14 text-sm" },
};

const clamp = (n: number, min: number, max: number) =>
  Math.min(max, Math.max(min, n));

/**
 * Ant Design `InputNumber` with steppers (cart / variant quantity). Buttons
 * disable at the bounds, typing clamps to [min, max], ArrowUp/ArrowDown step.
 */
export function QuantityPicker({
  value,
  defaultValue,
  min = 1,
  max = Number.POSITIVE_INFINITY,
  step = 1,
  onChange,
  disabled = false,
  label = "Số lượng",
  size = "md",
  className = "",
}: QuantityPickerProps) {
  const id = useId();
  const [internal, setInternal] = useState(
    clamp(defaultValue ?? min, min, max),
  );
  const current = value !== undefined ? clamp(value, min, max) : internal;
  // While the user is typing the field shows their draft, not the clamped value.
  const [draft, setDraft] = useState<string | null>(null);
  const s = sizes[size];
  // Latest committed value, updated synchronously so rapid events (key repeat,
  // batched updates) step from the right number rather than a stale render.
  const latest = useRef(current);
  latest.current = current;

  const commit = (next: number) => {
    const n = clamp(next, min, max);
    if (value === undefined) setInternal(n);
    if (n !== latest.current) onChange?.(n);
    latest.current = n;
    return n;
  };

  const onType = (raw: string) => {
    const digits = raw.replace(/[^\d]/g, "");
    if (digits === "") {
      setDraft("");
      return;
    }
    const typed = Number(digits);
    if (typed > max) {
      commit(max);
      setDraft(null);
      return;
    }
    setDraft(digits);
    if (typed >= min) commit(typed);
  };

  const settle = () => {
    if (draft === null) return;
    const typed = draft === "" ? min : Number(draft);
    commit(typed);
    setDraft(null);
  };

  return (
    <div
      className={`inline-flex overflow-hidden rounded-lg border border-border-subtle ${s.box} ${
        disabled ? "opacity-50 cursor-not-allowed" : ""
      } ${className}`}
    >
      <button
        type="button"
        aria-label="Giảm số lượng"
        aria-controls={id}
        disabled={disabled || current <= min}
        aria-disabled={disabled || current <= min ? "true" : undefined}
        onClick={() => {
          setDraft(null);
          commit(latest.current - step);
        }}
        className={`${stepButton} ${s.button} border-r border-border-subtle`}
      >
        <span aria-hidden="true">−</span>
      </button>
      <input
        id={id}
        type="text"
        inputMode="numeric"
        role="spinbutton"
        aria-label={label}
        aria-valuenow={current}
        aria-valuemin={min}
        aria-valuemax={Number.isFinite(max) ? max : undefined}
        aria-disabled={disabled ? "true" : undefined}
        disabled={disabled}
        value={draft ?? String(current)}
        onChange={(e) => onType(e.target.value)}
        onBlur={settle}
        onKeyDown={(e) => {
          if (e.key === "ArrowUp") {
            e.preventDefault();
            setDraft(null);
            commit(latest.current + step);
          } else if (e.key === "ArrowDown") {
            e.preventDefault();
            setDraft(null);
            commit(latest.current - step);
          } else if (e.key === "Enter") {
            settle();
          }
        }}
        className={`${s.input} bg-surface-card text-center text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-focus-ring disabled:pointer-events-none`}
      />
      <button
        type="button"
        aria-label="Tăng số lượng"
        aria-controls={id}
        disabled={disabled || current >= max}
        aria-disabled={disabled || current >= max ? "true" : undefined}
        onClick={() => {
          setDraft(null);
          commit(latest.current + step);
        }}
        className={`${stepButton} ${s.button} border-l border-border-subtle`}
      >
        <span aria-hidden="true">+</span>
      </button>
    </div>
  );
}
