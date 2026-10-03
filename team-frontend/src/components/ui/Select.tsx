import React from "react";
import { focusRing } from "./focus";

export interface SelectOption {
  value: string;
  label: React.ReactNode;
  disabled?: boolean;
}

export interface SelectProps
  extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "size"> {
  options?: SelectOption[];
  /** Disabled first option shown while nothing is selected. */
  placeholder?: string;
  selectSize?: "sm" | "md" | "lg";
  /** Error styling (also set aria-invalid yourself or use FormItem status="error"). */
  invalid?: boolean;
}

// Tier 3: component tokens.
const sizeStyles = {
  sm: "py-1.5 pl-3 pr-8 text-xs",
  md: "py-2 pl-3.5 pr-9 text-sm",
  lg: "py-2.5 pl-4 pr-10 text-base",
};

/**
 * Ant Design `Select` on a native `<select>` (native keyboard + mobile pickers).
 * Server-compatible; controlled or uncontrolled by the usual value/onChange.
 */
export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  (
    {
      options,
      placeholder,
      selectSize = "md",
      invalid = false,
      className = "",
      disabled,
      children,
      "aria-invalid": ariaInvalid,
      ...props
    },
    ref,
  ) => {
    const isInvalid = invalid || ariaInvalid === true || ariaInvalid === "true";
    // A placeholder needs a value to point at when the select is uncontrolled.
    const uncontrolledDefault =
      placeholder &&
      props.value === undefined &&
      props.defaultValue === undefined
        ? { defaultValue: "" }
        : {};
    return (
      <div className="relative w-full">
        <select
          ref={ref}
          disabled={disabled}
          aria-disabled={disabled ? "true" : undefined}
          aria-invalid={isInvalid ? "true" : undefined}
          className={`block w-full cursor-pointer appearance-none rounded-lg border bg-surface-card text-text-primary shadow-2xs transition duration-150 ${focusRing} disabled:cursor-not-allowed disabled:bg-surface-page disabled:opacity-50 disabled:pointer-events-none ${
            isInvalid
              ? "border-danger"
              : "border-border-subtle hover:border-border-strong"
          } ${sizeStyles[selectSize]} ${className}`}
          {...uncontrolledDefault}
          {...props}
        >
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options?.map((o) => (
            <option key={o.value} value={o.value} disabled={o.disabled}>
              {o.label}
            </option>
          ))}
          {children}
        </select>
        <svg
          aria-hidden="true"
          viewBox="0 0 20 20"
          className="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-text-disabled"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <path d="M5 8l5 5 5-5" />
        </svg>
      </div>
    );
  },
);

Select.displayName = "Select";
