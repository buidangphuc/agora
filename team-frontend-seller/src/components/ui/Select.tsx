import {
  NativeSelect,
  type NativeSelectOption,
  type NativeSelectProps,
} from "@fe-platform/ui";

export type SelectOption = NativeSelectOption;

export interface SelectProps
  extends Omit<NativeSelectProps, "errorText" | "helpText"> {
  helperText?: string;
  error?: string;
}

export function Select({ helperText, error, ...props }: SelectProps) {
  return <NativeSelect {...props} helpText={helperText} errorText={error} />;
}
