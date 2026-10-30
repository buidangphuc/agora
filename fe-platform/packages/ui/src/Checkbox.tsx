"use client";

import { components } from "@fe-platform/tokens";
import * as RadixCheckbox from "@radix-ui/react-checkbox";
import { useId } from "react";
import { cx } from "./cx";

const { checkbox, field } = components;

export type CheckboxProps = {
  label: string;
  labelHidden?: boolean;
  id?: string;
  name?: string;
  checked?: boolean | "indeterminate";
  defaultChecked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
  disabled?: boolean;
  errorText?: string;
  "data-testid"?: string;
};

/** Boolean choice with a label (visible unless `labelHidden`), optional mixed state and error text. */
export function Checkbox({
  label,
  labelHidden,
  id,
  name,
  checked,
  defaultChecked,
  onCheckedChange,
  disabled,
  errorText,
  "data-testid": testId,
}: CheckboxProps) {
  const auto = useId();
  const boxId = id ?? auto;
  const errorId = `${boxId}-error`;
  return (
    <div className="flex flex-col gap-1">
      <div className={checkbox.row}>
        <RadixCheckbox.Root
          id={boxId}
          name={name}
          checked={checked}
          defaultChecked={defaultChecked}
          onCheckedChange={(v) => onCheckedChange?.(v === true)}
          disabled={disabled}
          aria-invalid={errorText ? true : undefined}
          aria-describedby={errorText ? errorId : undefined}
          data-testid={testId}
          className={cx(
            checkbox.root,
            checkbox.checked,
            checked === "indeterminate" && checkbox.mixed,
          )}
        >
          <RadixCheckbox.Indicator aria-hidden="true">
            {checked === "indeterminate" ? "–" : "✓"}
          </RadixCheckbox.Indicator>
        </RadixCheckbox.Root>
        <label
          htmlFor={boxId}
          className={labelHidden ? "sr-only" : field.label}
        >
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
