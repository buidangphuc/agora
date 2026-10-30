"use client";

import { components } from "@fe-platform/tokens";
import * as RadixCheckbox from "@radix-ui/react-checkbox";
import { useId } from "react";
import { cx } from "./cx";

const { checkbox, field } = components;

export type CheckboxProps = {
  label: string;
  name?: string;
  checked?: boolean;
  defaultChecked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
  disabled?: boolean;
  errorText?: string;
};

/** Boolean choice with a visible label and optional error text. */
export function Checkbox({
  label,
  name,
  checked,
  defaultChecked,
  onCheckedChange,
  disabled,
  errorText,
}: CheckboxProps) {
  const id = useId();
  const errorId = `${id}-error`;
  return (
    <div className="flex flex-col gap-1">
      <div className={checkbox.row}>
        <RadixCheckbox.Root
          id={id}
          name={name}
          checked={checked}
          defaultChecked={defaultChecked}
          onCheckedChange={(v) => onCheckedChange?.(v === true)}
          disabled={disabled}
          aria-invalid={errorText ? true : undefined}
          aria-describedby={errorText ? errorId : undefined}
          className={cx(checkbox.root, checkbox.checked)}
        >
          <RadixCheckbox.Indicator aria-hidden="true">
            ✓
          </RadixCheckbox.Indicator>
        </RadixCheckbox.Root>
        <label htmlFor={id} className={field.label}>
          {label}
        </label>
      </div>
      {errorText && (
        <p id={errorId} className={field.error}>
          {errorText}
        </p>
      )}
    </div>
  );
}
