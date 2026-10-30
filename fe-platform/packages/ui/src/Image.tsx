import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { ImageClient } from "./ImageClient";
import { cx } from "./cx";

const { image } = components;

export type Aspect = keyof typeof image.aspect;

export type ImageProps = {
  src: string;
  alt: string;
  aspect: Aspect;
  fit?: "cover" | "contain";
  loading?: "lazy" | "eager";
  fallback?: ReactNode;
  fallbackLabel?: string;
  radius?: "xl" | "lg" | "none";
};

function EmptyPicture({ label }: { label: string }) {
  return (
    <div className={image.empty}>
      <svg
        viewBox="0 0 24 24"
        className={image.icon}
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        role="img"
        aria-label={label}
      >
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <circle cx="9" cy="10" r="1.5" />
        <path d="M21 16l-5-5-8 8" />
      </svg>
    </div>
  );
}

/** Picture in a fixed-aspect box that is reserved before load, lazy by default, with a fallback on error. */
export function Image({
  src,
  alt,
  aspect,
  fit = "cover",
  loading = "lazy",
  fallback,
  fallbackLabel = "No image",
  radius = "xl",
}: ImageProps) {
  return (
    <div className={cx(image.box, image.radius[radius], image.aspect[aspect])}>
      <ImageClient
        src={src}
        alt={alt}
        fit={fit}
        loading={loading}
        fallback={fallback ?? <EmptyPicture label={fallbackLabel} />}
      />
    </div>
  );
}
