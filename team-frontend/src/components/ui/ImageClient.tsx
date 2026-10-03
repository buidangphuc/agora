"use client";

import React, { useState } from "react";

export interface ImageClientProps {
  src: string;
  alt: string;
  loading?: "lazy" | "eager";
  fit?: "cover" | "contain";
  /** Shown instead of the picture when it fails to load. */
  fallback: React.ReactNode;
  className?: string;
}

/**
 * The only client part of Image / Avatar: tracks load / error so a broken URL
 * swaps to the fallback and a pulse placeholder shows until the picture loads.
 * Positioned absolutely: the parent owns the box (aspect ratio).
 */
export function ImageClient({
  src,
  alt,
  loading = "lazy",
  fit = "cover",
  fallback,
  className = "",
}: ImageClientProps) {
  const [status, setStatus] = useState<"loading" | "loaded" | "error">(
    "loading",
  );
  // A new src starts over.
  const [seen, setSeen] = useState(src);
  if (seen !== src) {
    setSeen(src);
    setStatus("loading");
  }

  if (status === "error") {
    return (
      <div className="absolute inset-0 flex items-center justify-center">
        {fallback}
      </div>
    );
  }

  return (
    <>
      {status === "loading" && (
        <div
          aria-hidden="true"
          className="absolute inset-0 animate-pulse bg-neutral-200"
        />
      )}
      <img
        src={src}
        alt={alt}
        loading={loading}
        onLoad={() => setStatus("loaded")}
        onError={() => setStatus("error")}
        className={`absolute inset-0 h-full w-full ${
          fit === "cover" ? "object-cover" : "object-contain"
        } ${className}`}
      />
    </>
  );
}
