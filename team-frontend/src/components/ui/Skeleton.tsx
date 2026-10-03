import React from "react";
import { type Aspect, aspectClass } from "./aspect";
import { range } from "./range";

export type SkeletonVariant = "text" | "avatar" | "image" | "card";

export interface SkeletonProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: SkeletonVariant;
  /** Text preset: number of lines (the last one is shorter). */
  lines?: number;
  /** Image / card preset: reserved aspect ratio. */
  aspect?: Aspect;
  /** Avatar preset size. */
  size?: "sm" | "md" | "lg";
}

// Tier 3: component tokens.
const block = "animate-pulse bg-neutral-200";
const avatarSize = { sm: "h-8 w-8", md: "h-10 w-10", lg: "h-14 w-14" };

function Lines({ lines }: { lines: number }) {
  return (
    <div className="space-y-2">
      {range(lines).map((row) => (
        <div
          key={row}
          className={`h-4 rounded-xs ${block} ${
            row === lines - 1 && lines > 1 ? "w-2/3" : "w-full"
          }`}
        />
      ))}
    </div>
  );
}

/**
 * Loading placeholder with the same footprint as the content it stands in
 * for. Server-compatible. Presets: text, avatar, image, card.
 */
export function Skeleton({
  variant = "text",
  lines = 3,
  aspect = "square",
  size = "md",
  className = "",
  ...props
}: SkeletonProps) {
  let body: React.ReactNode;
  if (variant === "avatar") {
    body = <div className={`rounded-full ${block} ${avatarSize[size]}`} />;
  } else if (variant === "image") {
    body = (
      <div className={`w-full rounded-xl ${block} ${aspectClass[aspect]}`} />
    );
  } else if (variant === "card") {
    body = (
      <div className="space-y-3 rounded-xl border border-border-subtle bg-surface-card p-3">
        <div className={`w-full rounded-lg ${block} ${aspectClass[aspect]}`} />
        <Lines lines={2} />
      </div>
    );
  } else {
    body = <Lines lines={lines} />;
  }

  return (
    <div
      aria-busy="true"
      data-variant={variant}
      className={className}
      {...props}
    >
      <span className="sr-only">Đang tải</span>
      {body}
    </div>
  );
}
