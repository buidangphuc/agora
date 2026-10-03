import React, { useId } from "react";
import { focusRing } from "./focus";

export interface RadioProps
  extends Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> {
  label?: React.ReactNode;
  description?: React.ReactNode;
}

/** Ant Design `Radio` on a native radio input. Server-compatible. */
export const Radio = React.forwardRef<HTMLInputElement, RadioProps>(
  ({ label, description, className = "", id, disabled, ...props }, ref) => {
    const autoId = useId();
    const inputId = id ?? autoId;
    return (
      <div
        className={`flex items-start gap-2.5 ${
          disabled ? "opacity-50 cursor-not-allowed" : ""
        } ${className}`}
      >
        <input
          ref={ref}
          id={inputId}
          type="radio"
          disabled={disabled}
          aria-disabled={disabled ? "true" : undefined}
          className={`mt-0.5 h-4 w-4 shrink-0 cursor-pointer border-border-strong accent-action-primary transition duration-150 ${focusRing} disabled:cursor-not-allowed disabled:pointer-events-none`}
          {...props}
        />
        {(label || description) && (
          <label
            htmlFor={inputId}
            className={`text-sm text-text-primary ${
              disabled ? "cursor-not-allowed" : "cursor-pointer"
            }`}
          >
            {label}
            {description && (
              <span className="block text-xs text-text-secondary">
                {description}
              </span>
            )}
          </label>
        )}
      </div>
    );
  },
);

Radio.displayName = "Radio";

export interface RadioOption {
  value: string;
  label: React.ReactNode;
  description?: React.ReactNode;
  disabled?: boolean;
}

export interface RadioGroupProps
  extends Omit<React.FieldsetHTMLAttributes<HTMLFieldSetElement>, "onChange"> {
  options: RadioOption[];
  /** Shared `name` of the radios (generated when omitted). */
  name?: string;
  /** Controlled value. */
  value?: string;
  /** Uncontrolled initial value. */
  defaultValue?: string;
  onChange?: (value: string) => void;
  disabled?: boolean;
  direction?: "vertical" | "horizontal";
  /** Visible group label (a `<legend>`); or name it with aria-label(ledby). */
  legend?: React.ReactNode;
}

/**
 * Ant Design `Radio.Group`: a `<fieldset>` of native radios sharing one name
 * (native arrow-key navigation). Server-compatible; no context needed.
 */
export function RadioGroup({
  options,
  name,
  value,
  defaultValue,
  onChange,
  disabled = false,
  direction = "vertical",
  legend,
  className = "",
  ...props
}: RadioGroupProps) {
  const autoName = useId();
  const groupName = name ?? autoName;
  return (
    <fieldset
      className={`min-w-0 border-0 p-0 m-0 ${className}`}
      disabled={disabled}
      aria-disabled={disabled ? "true" : undefined}
      {...props}
    >
      {legend && (
        <legend className="mb-1.5 text-xs font-medium text-text-primary">
          {legend}
        </legend>
      )}
      <div
        className={`flex gap-3 ${
          direction === "vertical" ? "flex-col" : "flex-row flex-wrap"
        }`}
      >
        {options.map((o) => (
          <Radio
            key={o.value}
            name={groupName}
            value={o.value}
            label={o.label}
            description={o.description}
            disabled={disabled || o.disabled}
            {...(value !== undefined
              ? { checked: value === o.value }
              : defaultValue !== undefined
                ? { defaultChecked: defaultValue === o.value }
                : {})}
            onChange={() => onChange?.(o.value)}
          />
        ))}
      </div>
    </fieldset>
  );
}
