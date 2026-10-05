"use client";

import React, { useEffect, useRef, useState } from "react";

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
  const imgRef = useRef<HTMLImageElement>(null);
  // A new src starts over.
  const [seen, setSeen] = useState(src);
  if (seen !== src) {
    setSeen(src);
    setStatus("loading");
  }

  // The server-rendered <img> may have loaded or failed before React hydrated,
  // in which case onLoad / onError never fire: read the result off the element.
  useEffect(() => {
    const img = imgRef.current;
    // Ignore a stale element that still shows the previous src.
    if (!img || img.getAttribute("src") !== src || !img.complete) return;
    setStatus(img.naturalWidth > 0 ? "loaded" : "error");
  }, [src]);

  // An empty src can never load: render the fallback in the first (server)
  // paint so a broken <img> never swaps for it later (layout shift).
  if (status === "error" || !src) {
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
        ref={imgRef}
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
