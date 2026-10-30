import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { tag, tone: tones } = components;

export type Tone = keyof typeof tones;

export type TagProps = NativeProps<"span"> & {
  tone?: Tone;
};

/** Small status or category label tinted by a tone; use ClosableTag when it can be dismissed. */
export function Tag({ tone = "neutral", children, ...rest }: TagProps) {
  return (
    <span {...rest} className={cx(tag.base, tones[tone])}>
      {children}
    </span>
  );
}
