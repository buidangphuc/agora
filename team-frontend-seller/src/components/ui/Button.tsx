import {
  Button as PlatformButton,
  type ButtonProps as PlatformButtonProps,
} from "@fe-platform/ui";

export type ButtonVariant =
  | "primary"
  | "secondary"
  | "outline"
  | "ghost"
  | "danger"
  | "white";

export type ButtonSize = "xs" | "sm" | "md" | "lg";

export interface ButtonProps
  extends Omit<PlatformButtonProps, "variant" | "loading"> {
  variant?: ButtonVariant;
  isLoading?: boolean;
}

const platformVariant = {
  primary: "primary",
  secondary: "secondary",
  outline: "secondary",
  white: "secondary",
  ghost: "ghost",
  danger: "danger",
} as const;

export function Button({
  variant = "primary",
  isLoading = false,
  disabled,
  ...props
}: ButtonProps) {
  const blocked = Boolean(disabled) || isLoading;
  return (
    <PlatformButton
      {...props}
      variant={platformVariant[variant]}
      loading={isLoading}
      disabled={disabled}
      aria-disabled={blocked ? "true" : undefined}
    />
  );
}
