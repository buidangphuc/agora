import React from "react";
import { Spinner } from "./Spin";

export type ButtonVariant =
  | "primary"
  | "secondary"
  | "outline"
  | "ghost"
  | "danger"
  | "white";

export type ButtonSize = "xs" | "sm" | "md" | "lg";

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  isLoading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

// Tier 3: component tokens, referencing Tier 2 aliases (Tier 1 scale only for
// the pressed fill, which has no alias).
const variantStyles: Record<ButtonVariant, string> = {
  primary:
    "bg-action-primary text-text-inverse hover:bg-action-primary-hover active:bg-primary-700 shadow-sm",
  secondary:
    "bg-surface-page text-text-primary hover:bg-border-subtle active:bg-border-strong",
  outline:
    "border border-border-strong text-text-primary bg-surface-card hover:bg-surface-muted active:bg-surface-page shadow-sm",
  ghost: "text-text-primary hover:bg-surface-page active:bg-border-subtle",
  danger:
    "bg-danger text-text-inverse hover:bg-accent-danger-dark active:bg-accent-danger-dark active:brightness-90 shadow-sm",
  white:
    "bg-surface-card text-text-primary hover:bg-surface-page active:bg-border-subtle shadow-sm",
};

const sizeStyles: Record<ButtonSize, string> = {
  xs: "py-1 px-2 text-xs rounded-lg",
  sm: "py-1.5 px-3 text-xs font-medium rounded-lg",
  md: "py-2 px-4 text-sm font-medium rounded-lg",
  lg: "py-2.5 px-5 text-base font-medium rounded-lg",
};

const gapStyles: Record<ButtonSize, string> = {
  xs: "gap-1",
  sm: "gap-1.5",
  md: "gap-2",
  lg: "gap-2.5",
};

const focusRing =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring focus-visible:ring-offset-2";

/**
 * Shared state contract (UI_SYSTEM_DESIGN.md section 4): hover, active scale,
 * focus-visible ring, disabled (native + aria-disabled) and loading
 * (`isLoading`: spinner overlay, label kept in flow so the width never changes,
 * activation blocked, aria-busy).
 */
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      children,
      className = "",
      variant = "primary",
      size = "md",
      isLoading = false,
      disabled = false,
      leftIcon,
      rightIcon,
      type = "button",
      onClick,
      ...props
    },
    ref,
  ) => {
    const blocked = disabled || isLoading;
    const stateStyles = disabled
      ? "opacity-50 pointer-events-none cursor-not-allowed"
      : isLoading
        ? "pointer-events-none cursor-progress"
        : "cursor-pointer active:scale-95";

    return (
      <button
        ref={ref}
        type={type}
        {...props}
        disabled={disabled}
        aria-disabled={blocked ? "true" : undefined}
        aria-busy={isLoading ? "true" : undefined}
        onClick={(e) => {
          if (blocked) {
            e.preventDefault();
            return;
          }
          onClick?.(e);
        }}
        className={`relative inline-flex items-center justify-center font-medium transition duration-150 select-none ${focusRing} ${stateStyles} ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      >
        <span
          className={`inline-flex items-center justify-center ${gapStyles[size]} ${
            isLoading ? "opacity-0" : ""
          }`}
        >
          {leftIcon && <span className="inline-flex shrink-0">{leftIcon}</span>}
          <span>{children}</span>
          {rightIcon && (
            <span className="inline-flex shrink-0">{rightIcon}</span>
          )}
        </span>
        {isLoading && (
          <span className="absolute inset-0 flex items-center justify-center">
            <Spinner />
          </span>
        )}
      </button>
    );
  },
);

Button.displayName = "Button";
