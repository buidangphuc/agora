"use client";

import { components } from "@fe-platform/tokens";
import * as RadixSelect from "@radix-ui/react-select";
import { useId } from "react";
import { cx } from "./cx";

const { select, field, input } = components;

export type SelectOption = { value: string; label: string };

export type SelectProps = {
  label: string;
  options: SelectOption[];
  placeholder?: string;
  name?: string;
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  disabled?: boolean;
  errorText?: string;
};

/** Single choice from a short list; use Input for free text. */
export function Select({
  label,
  options,
  placeholder = "Select an option",
  name,
  value,
  defaultValue,
  onValueChange,
  disabled,
  errorText,
}: SelectProps) {
  const id = useId();
  const labelId = `${id}-label`;
  const errorId = `${id}-error`;
  return (
    <div className="flex flex-col gap-1">
      <span id={labelId} className={field.label}>
        {label}
      </span>
      <RadixSelect.Root
        name={name}
        value={value}
        defaultValue={defaultValue}
        onValueChange={onValueChange}
        disabled={disabled}
      >
        <RadixSelect.Trigger
          aria-labelledby={labelId}
          aria-invalid={errorText ? true : undefined}
          aria-describedby={errorText ? errorId : undefined}
          className={cx(
            select.trigger,
            errorText && input.invalid,
            disabled && input.disabled,
          )}
        >
          <RadixSelect.Value placeholder={placeholder} />
          <RadixSelect.Icon aria-hidden="true">▾</RadixSelect.Icon>
        </RadixSelect.Trigger>
        <RadixSelect.Portal>
          <RadixSelect.Content position="popper" className={select.content}>
            <RadixSelect.Viewport>
              {options.map((o) => (
                <RadixSelect.Item
                  key={o.value}
                  value={o.value}
                  className={select.item}
                >
                  <RadixSelect.ItemText>{o.label}</RadixSelect.ItemText>
                </RadixSelect.Item>
              ))}
            </RadixSelect.Viewport>
          </RadixSelect.Content>
        </RadixSelect.Portal>
      </RadixSelect.Root>
      {errorText && (
        <p id={errorId} className={field.error}>
          {errorText}
        </p>
      )}
    </div>
  );
}
