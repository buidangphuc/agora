import React from "react";
import { ImageClient } from "./ImageClient";

export interface AvatarProps {
  src?: string;
  /** Used for the image alt text and to derive the initials fallback. */
  name?: string;
  size?: "sm" | "md" | "lg" | "xl";
  shape?: "circle" | "square";
  className?: string;
}

// Tier 3: component tokens.
const sizes = {
  sm: "h-8 w-8 text-xs",
  md: "h-10 w-10 text-sm",
  lg: "h-14 w-14 text-base",
  xl: "h-20 w-20 text-xl",
};

/** Up to two initials of a name ("Nguyễn Văn A" -> "NA"); "?" when empty. */
export function initials(name?: string): string {
  const words = (name ?? "").trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  const first = words[0][0];
  const last = words.length > 1 ? words[words.length - 1][0] : "";
  return (first + last).toUpperCase();
}

/**
 * Ant Design `Avatar`: a picture with an initials fallback (on missing src or
 * a failed load). Server-compatible; only the picture is a client island.
 */
export function Avatar({
  src,
  name,
  size = "md",
  shape = "circle",
  className = "",
}: AvatarProps) {
  const text = (
    <span className="font-semibold text-text-secondary">{initials(name)}</span>
  );
  return (
    <span
      role={src ? undefined : "img"}
      aria-label={src ? undefined : name || "Người dùng"}
      className={`relative inline-flex shrink-0 select-none items-center justify-center overflow-hidden bg-surface-page ${
        shape === "circle" ? "rounded-full" : "rounded-lg"
      } ${sizes[size]} ${className}`}
    >
      {src ? (
        <ImageClient src={src} alt={name ?? ""} fallback={text} />
      ) : (
        <span aria-hidden="true">{text}</span>
      )}
    </span>
  );
}
