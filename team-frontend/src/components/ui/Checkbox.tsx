import React, { useId } from "react";
import { focusRing } from "./focus";

export interface CheckboxProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> {
  label?: React.ReactNode;
  /** Secondary text under the label. */
  description?: React.ReactNode;
}

/** Ant Design `Checkbox` on a native checkbox. Server-compatible. */
export const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ label, description, className = "", id, disabled, ...props }, ref) => {
    const autoId = useId();
    const inputId = id ?? autoId;
    return (
      <div
        className={`flex items-start gap-2.5 ${
          disabled ? "opacity-50 cursor-not-allowed" : ""
        } ${className}`}
      >
        <input
          ref={ref}
          id={inputId}
          type="checkbox"
          disabled={disabled}
          aria-disabled={disabled ? "true" : undefined}
          className={`mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded-xs border-border-strong accent-action-primary transition duration-150 ${focusRing} disabled:cursor-not-allowed disabled:pointer-events-none`}
          {...props}
        />
        {(label || description) && (
          <label
            htmlFor={inputId}
            className={`text-sm text-text-primary ${
              disabled ? "cursor-not-allowed" : "cursor-pointer"
            }`}
          >
            {label}
            {description && (
              <span className="block text-xs text-text-secondary">
                {description}
              </span>
            )}
          </label>
        )}
      </div>
    );
  },
);

Checkbox.displayName = "Checkbox";
