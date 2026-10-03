import React from "react";

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  helperText?: string;
  error?: string;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  inputSize?: "sm" | "md" | "lg";
}

const sizePadding: Record<"sm" | "md" | "lg", string> = {
  sm: "py-1.5 px-3 text-xs rounded-md",
  md: "py-2 px-3.5 text-sm rounded-lg",
  lg: "py-2.5 px-4 text-base rounded-lg",
};

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
      ...props
    },
    ref,
  ) => {
    const inputId =
      id || (label ? label.toLowerCase().replace(/\s+/g, "-") : undefined);

    return (
      <div className="w-full space-y-1.5">
        {label && (
          <label
            htmlFor={inputId}
            className="block text-xs font-medium text-gray-700"
          >
            {label}
          </label>
        )}
        <div className="relative rounded-lg shadow-2xs">
          {leftIcon && (
            <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none text-gray-400">
              {leftIcon}
            </div>
          )}
          <input
            ref={ref}
            id={inputId}
            disabled={disabled}
            className={`block w-full border bg-white text-gray-900 transition placeholder:text-gray-400 focus:outline-none focus:ring-2 focus:ring-offset-0 disabled:bg-gray-100 disabled:opacity-60 disabled:cursor-not-allowed ${
              error
                ? "border-red-300 focus:border-red-500 focus:ring-red-200"
                : "border-gray-200 focus:border-primary-500 focus:ring-primary-100"
            } ${sizePadding[inputSize]} ${leftIcon ? "pl-9" : ""} ${
              rightIcon ? "pr-9" : ""
            } ${className}`}
            {...props}
          />
          {rightIcon && (
            <div className="absolute inset-y-0 right-0 pr-3 flex items-center pointer-events-none text-gray-400">
              {rightIcon}
            </div>
          )}
        </div>
        {error ? (
          <p className="text-xs text-red-600 font-medium">{error}</p>
        ) : helperText ? (
          <p className="text-xs text-gray-500">{helperText}</p>
        ) : null}
      </div>
    );
  },
);

Input.displayName = "Input";
