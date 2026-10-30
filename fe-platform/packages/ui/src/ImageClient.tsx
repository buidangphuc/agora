"use client";

import { components } from "@fe-platform/tokens";
import { type ReactNode, useEffect, useRef, useState } from "react";
import { cx } from "./cx";

const { image } = components;

export type ImageClientProps = {
  src: string;
  alt: string;
  loading?: "lazy" | "eager";
  fit?: "cover" | "contain";
  fallback: ReactNode;
};

export function ImageClient({
  src,
  alt,
  loading = "lazy",
  fit = "cover",
  fallback,
}: ImageClientProps) {
  const [status, setStatus] = useState<"loading" | "loaded" | "error">(
    "loading",
  );
  const imgRef = useRef<HTMLImageElement>(null);
  const [seen, setSeen] = useState(src);
  if (seen !== src) {
    setSeen(src);
    setStatus("loading");
  }

  useEffect(() => {
    const img = imgRef.current;
    if (!img || img.getAttribute("src") !== src || !img.complete) return;
    setStatus(img.naturalWidth > 0 ? "loaded" : "error");
  }, [src]);

  if (status === "error" || !src) {
    return <div className={image.fallback}>{fallback}</div>;
  }

  return (
    <>
      {status === "loading" && (
        <div aria-hidden="true" className={image.placeholder} />
      )}
      <img
        ref={imgRef}
        src={src}
        alt={alt}
        loading={loading}
        onLoad={() => setStatus("loaded")}
        onError={() => setStatus("error")}
        className={cx(image.picture, image.fit[fit])}
      />
    </>
  );
}
