import React, { useId } from "react";

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  helperText?: string;
  error?: string;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  inputSize?: "sm" | "md" | "lg";
}

// Tier 3: component tokens.
const sizePadding: Record<"sm" | "md" | "lg", string> = {
  sm: "py-1.5 px-3 text-xs rounded-lg",
  md: "py-2 px-3.5 text-sm rounded-lg",
  lg: "py-2.5 px-4 text-base rounded-lg",
};

const toneStyles = {
  default: "border-border-subtle focus-visible:border-action-primary",
  error: "border-danger focus-visible:border-danger",
};

/**
 * Text input (Ant `Input`). The label is bound with `htmlFor`; helper and error
 * text are linked through `aria-describedby`; an error sets `aria-invalid`.
 */
export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  (
    {
      label,
      helperText,
      error,
      leftIcon,
      rightIcon,
      inputSize = "md",
      className = "",
      id,
      disabled,
      required,
      "aria-describedby": describedByProp,
      "aria-invalid": ariaInvalidProp,
      ...props
    },
    ref,
  ) => {
    const autoId = useId();
    const inputId =
      id || (label ? label.toLowerCase().replace(/\s+/g, "-") : autoId);
    const messageId = error
      ? `${inputId}-error`
      : helperText
        ? `${inputId}-help`
        : undefined;
    const describedBy =
      [describedByProp, messageId].filter(Boolean).join(" ") || undefined;

    return (
      <div className="w-full space-y-1.5">
        {label && (
          <label
            htmlFor={inputId}
            className="block text-xs font-medium text-text-primary"
          >
            {label}
            {required && (
              <span className="ml-0.5 text-danger" aria-hidden="true">
                *
              </span>
            )}
          </label>
        )}
        <div className="relative rounded-lg shadow-2xs">
          {leftIcon && (
            <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-text-disabled">
              {leftIcon}
            </div>
          )}
          <input
            ref={ref}
            id={inputId}
            disabled={disabled}
            required={required}
            aria-disabled={disabled ? "true" : undefined}
            aria-invalid={error ? "true" : ariaInvalidProp}
            aria-describedby={describedBy}
            className={`block w-full border bg-surface-card text-text-primary transition duration-150 placeholder:text-text-disabled focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring disabled:bg-surface-page disabled:opacity-50 disabled:cursor-not-allowed disabled:pointer-events-none ${
              error ? toneStyles.error : toneStyles.default
            } ${sizePadding[inputSize]} ${leftIcon ? "pl-9" : ""} ${
              rightIcon ? "pr-9" : ""
            } ${className}`}
            {...props}
          />
          {rightIcon && (
            <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none text-text-disabled">
              {rightIcon}
            </div>
          )}
        </div>
        {error ? (
          <p
            id={messageId}
            aria-live="polite"
            className="text-xs text-danger font-medium"
          >
            {error}
          </p>
        ) : helperText ? (
          <p id={messageId} className="text-xs text-text-secondary">
            {helperText}
          </p>
        ) : null}
      </div>
    );
  },
);

Input.displayName = "Input";
