import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { link, button } = components;

export type LinkProps = NativeProps<"a"> & {
  href: string;
  prefetch?: boolean;
  replace?: boolean;
  scroll?: boolean;
  variant?: "inline" | "subtle" | keyof typeof button.variant;
  size?: keyof typeof button.size;
  fullWidth?: boolean | "mobile";
};

function classes(
  variant: NonNullable<LinkProps["variant"]>,
  size: NonNullable<LinkProps["size"]>,
  fullWidth: LinkProps["fullWidth"],
) {
  if (variant === "inline") return link.base;
  if (variant === "subtle") return link.subtle;
  return cx(
    button.base,
    button.variant[variant],
    button.size[size],
    fullWidth === "mobile"
      ? button.fullWidthMobile
      : fullWidth && button.fullWidth,
  );
}

/** Navigation to another page with Next client routing. `inline` is an underlined text link, `subtle` keeps the surrounding text colour, and the Button variants draw a button-shaped link. Use Button for actions on the current page. */
export function Link({
  variant = "inline",
  size = "md",
  fullWidth = false,
  ...rest
}: LinkProps) {
  return <NextLink {...rest} className={classes(variant, size, fullWidth)} />;
}
