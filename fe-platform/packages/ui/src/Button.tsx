import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { button } = components;

export type ButtonProps = NativeProps<"button"> & {
  variant?: keyof typeof button.variant;
  size?: keyof typeof button.size;
  loading?: boolean;
  fullWidth?: boolean;
  leftIcon?: ReactNode;
  rightIcon?: ReactNode;
};

/** Action trigger. One `primary` per view; `ghost` for low-emphasis actions; `danger` for destructive ones. */
export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  fullWidth = false,
  leftIcon,
  rightIcon,
  disabled,
  type = "button",
  children,
  ...rest
}: ButtonProps) {
  const inactive = disabled || loading;
  return (
    <button
      {...rest}
      type={type}
      disabled={inactive}
      aria-busy={loading || undefined}
      className={cx(
        button.base,
        button.variant[variant],
        button.size[size],
        fullWidth && button.fullWidth,
        inactive && button.disabled,
      )}
    >
      {loading && <span aria-hidden="true" className={button.spinner} />}
      {leftIcon && <span className={button.icon}>{leftIcon}</span>}
      {children}
      {rightIcon && <span className={button.icon}>{rightIcon}</span>}
    </button>
  );
}
