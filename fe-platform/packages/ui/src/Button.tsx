import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { button } = components;

export type ButtonProps = NativeProps<"button"> & {
  variant?: keyof typeof button.variant;
  size?: keyof typeof button.size;
  loading?: boolean;
};

/** Action trigger. One `primary` per view; `ghost` for low-emphasis actions. */
export function Button({
  variant = "primary",
  size = "md",
  loading = false,
  disabled,
  type = "button",
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
        inactive && button.disabled,
      )}
    />
  );
}
