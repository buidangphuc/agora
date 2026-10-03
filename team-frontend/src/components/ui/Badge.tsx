import React from "react";

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

const variantStyles: Record<BadgeVariant, string> = {
  primary:
    "bg-primary-50 text-primary-600 border border-primary-200/60 font-semibold",
  mall: "bg-danger text-white font-bold tracking-wider uppercase",
  success:
    "bg-emerald-50 text-emerald-700 border border-emerald-200 font-medium",
  warning: "bg-amber-50 text-amber-700 border border-amber-200 font-medium",
  danger: "bg-rose-50 text-rose-700 border border-rose-200 font-medium",
  neutral: "bg-gray-100 text-gray-700 border border-gray-200 font-medium",
  discount: "bg-amber-300 text-primary-600 font-black",
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
  const roundedClass = pill ? "rounded-full" : "rounded-md";

  return (
    <span
      className={`inline-flex items-center justify-center select-none ${roundedClass} ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
      {...props}
    >
      {children}
    </span>
  );
}
