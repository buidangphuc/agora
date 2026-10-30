import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { skeleton, pageSkeleton } = components;

export type SkeletonVariant = "text" | "avatar" | "image" | "card";

export type SkeletonAspect = keyof typeof skeleton.aspect;

export type SkeletonProps = NativeProps<"div"> & {
  variant?: SkeletonVariant;
  lines?: number;
  aspect?: SkeletonAspect;
  size?: "sm" | "md" | "lg";
  label?: string;
};

function Lines({ lines }: { lines: number }) {
  const rows = Array.from({ length: Math.max(0, lines) }, (_, i) => i);
  return (
    <div className={skeleton.lines}>
      {rows.map((row) => (
        <div
          key={row}
          className={cx(
            skeleton.line,
            skeleton.block,
            row === lines - 1 && lines > 1 ? skeleton.short : skeleton.full,
          )}
        />
      ))}
    </div>
  );
}

/** Busy placeholder that keeps the footprint of the text, avatar, image or card it stands in for. */
export function Skeleton({
  variant = "text",
  lines = 3,
  aspect = "square",
  size = "md",
  label = "Loading",
  ...rest
}: SkeletonProps) {
  let body: ReactNode;
  if (variant === "avatar") {
    body = (
      <div
        className={cx(skeleton.round, skeleton.block, skeleton.avatar[size])}
      />
    );
  } else if (variant === "image") {
    body = (
      <div
        className={cx(skeleton.image, skeleton.block, skeleton.aspect[aspect])}
      />
    );
  } else if (variant === "card") {
    body = (
      <div className={skeleton.card}>
        <div
          className={cx(
            skeleton.cardMedia,
            skeleton.block,
            skeleton.aspect[aspect],
          )}
        />
        <Lines lines={2} />
      </div>
    );
  } else {
    body = <Lines lines={lines} />;
  }

  return (
    <div {...rest} aria-busy="true" data-variant={variant}>
      <span className="sr-only">{label}</span>
      {body}
    </div>
  );
}

const TILES = Array.from({ length: 12 }, (_, i) => `t${i + 1}`);

export type PageSkeletonProps = NativeProps<"div"> & { label?: string };

/** Generic page placeholder (heading, hero and tile grid) for routes without a bespoke loading state. */
export function PageSkeleton({
  label = "Loading",
  ...rest
}: PageSkeletonProps) {
  return (
    <div
      {...rest}
      aria-busy="true"
      aria-live="polite"
      className={pageSkeleton.root}
    >
      <span className="sr-only">{label}</span>
      <div className={pageSkeleton.heading} />
      <div className={pageSkeleton.hero} />
      <div className={pageSkeleton.grid}>
        {TILES.map((id) => (
          <div key={id} className={pageSkeleton.tile} />
        ))}
      </div>
    </div>
  );
}
