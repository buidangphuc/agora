import {
  Input as PlatformInput,
  type InputProps as PlatformInputProps,
} from "@fe-platform/ui";

export interface InputProps
  extends Omit<PlatformInputProps, "errorText" | "helpText"> {
  helperText?: string;
  error?: string;
}

export function Input({ helperText, error, ...props }: InputProps) {
  return <PlatformInput {...props} helpText={helperText} errorText={error} />;
}
