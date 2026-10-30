import { components } from "@fe-platform/tokens";
import { useId } from "react";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { input, field } = components;

export type InputProps = NativeProps<"input"> & {
  label: string;
  errorText?: string;
  helpText?: string;
};

/** Text field with a required visible label; `required` adds a hidden-from-AT mark. Errors are text, never colour alone. */
export function Input({
  label,
  errorText,
  helpText,
  id,
  disabled,
  required,
  ...rest
}: InputProps) {
  const auto = useId();
  const inputId = id ?? auto;
  const errorId = `${inputId}-error`;
  const helpId = `${inputId}-help`;
  const describedBy =
    [errorText && errorId, helpText && helpId].filter(Boolean).join(" ") ||
    undefined;
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={inputId} className={field.label}>
        {label}
        {required && (
          <span aria-hidden="true" className={field.required}>
            *
          </span>
        )}
      </label>
      <input
        {...rest}
        id={inputId}
        disabled={disabled}
        required={required}
        aria-invalid={errorText ? true : undefined}
        aria-describedby={describedBy}
        className={cx(
          input.base,
          errorText && input.invalid,
          disabled && input.disabled,
        )}
      />
      {helpText && (
        <p id={helpId} className={field.help}>
          {helpText}
        </p>
      )}
      {errorText && (
        <p id={errorId} className={field.error}>
          {errorText}
        </p>
      )}
    </div>
  );
}
