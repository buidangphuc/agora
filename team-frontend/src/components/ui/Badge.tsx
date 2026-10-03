import React from "react";
import { toneStyles } from "./tones";

export type BadgeVariant =
  | "primary"
  | "mall"
  | "success"
  | "warning"
  | "danger"
  | "neutral"
  | "discount";

export type BadgeSize = "xs" | "sm" | "md";

export interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  size?: BadgeSize;
  pill?: boolean;
}

// Tier 3: component tokens. Count / dot / label roles; new label code uses Tag.
const variantStyles: Record<BadgeVariant, string> = {
  primary: `${toneStyles.primary.soft} border font-semibold`,
  mall: "bg-danger text-text-inverse font-bold tracking-wider uppercase",
  success: `${toneStyles.success.soft} border font-medium`,
  warning: `${toneStyles.warning.soft} border font-medium`,
  danger: `${toneStyles.danger.soft} border font-medium`,
  neutral: `${toneStyles.neutral.soft} border font-medium`,
  discount: "bg-promo text-action-primary font-black",
};

const sizeStyles: Record<BadgeSize, string> = {
  xs: "py-0.5 px-1.5 text-xs leading-none",
  sm: "py-0.5 px-2 text-xs leading-4",
  md: "py-1 px-2.5 text-xs font-medium leading-4",
};

export function Badge({
  children,
  className = "",
  variant = "primary",
  size = "sm",
  pill = false,
  ...props
}: BadgeProps) {
  const roundedClass = pill ? "rounded-full" : "rounded-lg";

  return (
    <span
      className={`inline-flex items-center justify-center select-none ${roundedClass} ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      {...props}
    >
      {children}
    </span>
  );
}
