import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { badge, tone: tones } = components;

export type BadgeTone = keyof typeof badge.solid;

export type BadgeProps = NativeProps<"span"> & {
  tone?: BadgeTone;
  solid?: boolean;
  size?: "sm" | "md";
  pill?: boolean;
};

/** Compact count or marker such as a filter count or a promotion flag; use Tag for labels. */
export function Badge({
  tone = "primary",
  solid = false,
  size = "sm",
  pill = false,
  children,
  ...rest
}: BadgeProps) {
  return (
    <span
      {...rest}
      className={cx(
        badge.base,
        pill ? badge.shape.pill : badge.shape.square,
        badge.size[size],
        solid ? badge.solid[tone] : tones[tone],
      )}
    >
      {children}
    </span>
  );
}
