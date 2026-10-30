import {
  type BadgeTone,
  Badge as PlatformBadge,
  type BadgeProps as PlatformBadgeProps,
} from "@fe-platform/ui";

export type BadgeVariant =
  | "primary"
  | "mall"
  | "success"
  | "warning"
  | "danger"
  | "neutral"
  | "discount";

export type BadgeSize = "xs" | "sm" | "md";

export interface BadgeProps
  extends Omit<PlatformBadgeProps, "tone" | "solid" | "size"> {
  variant?: BadgeVariant;
  size?: BadgeSize;
}

const variants: Record<BadgeVariant, { tone: BadgeTone; solid: boolean }> = {
  primary: { tone: "primary", solid: false },
  mall: { tone: "danger", solid: true },
  success: { tone: "success", solid: false },
  warning: { tone: "warning", solid: false },
  danger: { tone: "danger", solid: false },
  neutral: { tone: "neutral", solid: false },
  discount: { tone: "warning", solid: true },
};

export function Badge({
  variant = "primary",
  size = "sm",
  ...props
}: BadgeProps) {
  const { tone, solid } = variants[variant];
  return (
    <PlatformBadge
      {...props}
      tone={tone}
      solid={solid}
      size={size === "md" ? "md" : "sm"}
    />
  );
}
