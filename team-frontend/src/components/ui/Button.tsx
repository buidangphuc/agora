import React from "react";

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

const variantStyles: Record<ButtonVariant, string> = {
  primary:
    "bg-primary-500 text-white hover:bg-primary-600 active:bg-primary-700 focus:ring-primary-500 shadow-sm",
  secondary:
    "bg-gray-100 text-gray-800 hover:bg-gray-200 active:bg-gray-300 focus:ring-gray-400",
  outline:
    "border border-gray-300 text-gray-700 bg-white hover:bg-gray-50 active:bg-gray-100 focus:ring-primary-500 shadow-sm",
  ghost:
    "text-gray-700 hover:bg-gray-100 active:bg-gray-200 focus:ring-gray-400",
  danger:
    "bg-red-600 text-white hover:bg-red-700 active:bg-red-800 focus:ring-red-500 shadow-sm",
  white:
    "bg-white text-gray-800 hover:bg-gray-100 active:bg-gray-200 focus:ring-gray-400 shadow-sm",
};

const sizeStyles: Record<ButtonSize, string> = {
  xs: "py-1 px-2 text-xs rounded-md gap-1",
  sm: "py-1.5 px-3 text-xs font-medium rounded-md gap-1.5",
  md: "py-2 px-4 text-sm font-medium rounded-lg gap-2",
  lg: "py-2.5 px-5 text-base font-medium rounded-lg gap-2.5",
};

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
      ...props
    },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        disabled={disabled || isLoading}
        className={`inline-flex items-center justify-center font-medium transition-all duration-150 focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:pointer-events-none cursor-pointer select-none ${variantStyles[variant]} ${sizeStyles[size]} ${className}`}
        {...props}
      >
        {isLoading ? (
          <span className="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent mr-1.5" />
        ) : (
          leftIcon && <span className="inline-flex shrink-0">{leftIcon}</span>
        )}
        <span>{children}</span>
        {!isLoading && rightIcon && (
          <span className="inline-flex shrink-0">{rightIcon}</span>
        )}
      </button>
    );
  },
);

Button.displayName = "Button";
