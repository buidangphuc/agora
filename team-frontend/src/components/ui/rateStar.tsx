import React from "react";

export const rateSizes = { sm: "h-4 w-4", md: "h-5 w-5", lg: "h-6 w-6" };

const starPath =
  "M10 1.5l2.6 5.3 5.9.9-4.25 4.1 1 5.8L10 14.9l-5.25 2.7 1-5.8L1.5 7.7l5.9-.9z";

function Glyph({ className }: { className: string }) {
  return (
    <svg
      viewBox="0 0 20 20"
      aria-hidden="true"
      className={`shrink-0 ${className}`}
    >
      <path d={starPath} fill="currentColor" />
    </svg>
  );
}

/** One star filled by `fill` (0, 0.5 or 1): an empty star with a clipped filled overlay. */
export function RateStar({
  fill,
  size,
  className = "",
}: {
  fill: number;
  size: keyof typeof rateSizes;
  className?: string;
}) {
  const dim = rateSizes[size];
  return (
    <span className={`relative inline-flex ${dim} ${className}`}>
      <span className="text-border-strong">
        <Glyph className={dim} />
      </span>
      {fill > 0 && (
        <span
          className={`absolute inset-y-0 left-0 overflow-hidden text-promo ${
            fill >= 1 ? "w-full" : "w-1/2"
          }`}
        >
          <Glyph className={dim} />
        </span>
      )}
    </span>
  );
}
