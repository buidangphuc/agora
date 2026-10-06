import React from "react";
import { ImageClient } from "./ImageClient";
import { type Aspect, aspectClass } from "./aspect";

export interface ImageProps {
  src: string;
  alt: string;
  /** Required: the box is reserved before the picture loads (CLS = 0). */
  aspect: Aspect;
  fit?: "cover" | "contain";
  /** Defaults to "lazy"; pass "eager" for above-the-fold images. */
  loading?: "lazy" | "eager";
  /** Replaces the default placeholder when the URL fails to load. */
  fallback?: React.ReactNode;
  className?: string;
}

function DefaultFallback() {
  return (
    <div className="flex h-full w-full items-center justify-center bg-surface-page text-text-disabled">
      <svg
        viewBox="0 0 24 24"
        className="h-8 w-8"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        role="img"
        aria-label="Không có ảnh"
      >
        <rect x="3" y="4" width="18" height="16" rx="2" />
        <circle cx="9" cy="10" r="1.5" />
        <path d="M21 16l-5-5-8 8" />
      </svg>
    </div>
  );
}

/**
 * Ant Design `Image`: a fixed-aspect box (server-rendered, so the space is
 * reserved before any JavaScript runs) around a client picture that falls back
 * on error and is lazy by default.
 */
export function Image({
  src,
  alt,
  aspect,
  fit = "cover",
  loading = "lazy",
  fallback,
  className = "",
}: ImageProps) {
  return (
    <div
      className={`relative w-full overflow-hidden rounded-xl bg-surface-page ${aspectClass[aspect]} ${className}`}
    >
      <ImageClient
        src={src}
        alt={alt}
        fit={fit}
        loading={loading}
        fallback={fallback ?? <DefaultFallback />}
      />
    </div>
  );
}
