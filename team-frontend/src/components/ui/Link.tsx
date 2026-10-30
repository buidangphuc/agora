import {
  Link as PlatformLink,
  type LinkProps as PlatformLinkProps,
} from "@fe-platform/ui";

import type { ButtonVariant } from "./Button";

export type LinkVariant = "inline" | "subtle" | ButtonVariant;

export interface LinkProps extends Omit<PlatformLinkProps, "variant"> {
  variant?: LinkVariant;
}

const platformVariant = {
  inline: "inline",
  subtle: "subtle",
  primary: "primary",
  secondary: "secondary",
  outline: "secondary",
  white: "secondary",
  ghost: "ghost",
  danger: "danger",
} as const;

export function Link({ variant = "inline", ...props }: LinkProps) {
  return <PlatformLink {...props} variant={platformVariant[variant]} />;
}
