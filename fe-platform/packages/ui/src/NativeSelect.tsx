import { components } from "@fe-platform/tokens";
import { useId } from "react";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { select, input, field } = components;

export type NativeSelectOption = {
  value: string;
  label: string;
  disabled?: boolean;
};

export type NativeSelectProps = Omit<NativeProps<"select">, "children"> & {
  label: string;
  labelHidden?: boolean;
  options: NativeSelectOption[];
  placeholder?: string;
  errorText?: string;
  helpText?: string;
};

/** Single choice on a real `<select>` (native pickers, form submit, server-renderable); use Select for a styled popup list. */
export function NativeSelect({
  label,
  labelHidden,
  options,
  placeholder,
  errorText,
  helpText,
  id,
  disabled,
  required,
  "aria-describedby": extraDescribedBy,
  ...rest
}: NativeSelectProps) {
  const auto = useId();
  const selectId = id ?? auto;
  const errorId = `${selectId}-error`;
  const helpId = `${selectId}-help`;
  const describedBy =
    [extraDescribedBy, errorText && errorId, helpText && helpId]
      .filter(Boolean)
      .join(" ") || undefined;
  const placeholderDefault =
    placeholder && rest.value === undefined && rest.defaultValue === undefined
      ? { defaultValue: "" }
      : {};
  return (
    <div className="flex flex-col gap-1">
      <label
        htmlFor={selectId}
        className={labelHidden ? "sr-only" : field.label}
      >
        {label}
        {required && !labelHidden && (
          <span aria-hidden="true" className={field.required}>
            *
          </span>
        )}
      </label>
      <div className="relative">
        <select
          {...placeholderDefault}
          {...rest}
          id={selectId}
          disabled={disabled}
          required={required}
          aria-invalid={errorText ? true : undefined}
          aria-describedby={describedBy}
          className={cx(
            select.native,
            errorText && input.invalid,
            disabled && input.disabled,
          )}
        >
          {placeholder && (
            <option value="" disabled>
              {placeholder}
            </option>
          )}
          {options.map((o) => (
            <option key={o.value} value={o.value} disabled={o.disabled}>
              {o.label}
            </option>
          ))}
        </select>
        <span aria-hidden="true" className={select.icon}>
          ▾
        </span>
      </div>
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
