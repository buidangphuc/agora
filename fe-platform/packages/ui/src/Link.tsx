import { components } from "@fe-platform/tokens";
import NextLink from "next/link";
import type { NativeProps } from "./types";

const { link } = components;

export type LinkProps = NativeProps<"a"> & {
  href: string;
  prefetch?: boolean;
  replace?: boolean;
  scroll?: boolean;
};

/** Navigation to another page. Use for every "go there" action; Button is for actions on the current page. */
export function Link(props: LinkProps) {
  return <NextLink {...props} className={link.base} />;
}
