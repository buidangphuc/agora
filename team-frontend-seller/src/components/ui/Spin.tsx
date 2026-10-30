import React from "react";

export type SpinSize = "sm" | "md" | "lg";

// Tier 3: component tokens (aliases only).
const sizeStyles: Record<SpinSize, string> = {
  sm: "h-4 w-4 border-2",
  md: "h-6 w-6 border-2",
  lg: "h-10 w-10 border-4",
};

/** Bare spinner glyph; inherits the text colour (used inside Button). */
export function Spinner({
  size = "sm",
  className = "",
}: {
  size?: SpinSize;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={`inline-block shrink-0 animate-spin rounded-full border-current border-t-transparent ${sizeStyles[size]} ${className}`}
    />
  );
}

export interface SpinProps extends React.HTMLAttributes<HTMLDivElement> {
  size?: SpinSize;
  spinning?: boolean;
  /** Text shown beside the spinner and used as its accessible name. */
  tip?: string;
}

/**
 * Ant Design `Spin`. Standalone it is a centred spinner; with children it
 * overlays them while `spinning`, keeping their footprint (no layout shift).
 */
export function Spin({
  size = "md",
  spinning = true,
  tip,
  children,
  className = "",
  ...props
}: SpinProps) {
  const indicator = (
    <output
      aria-live="polite"
      className="inline-flex items-center gap-2 text-action-primary"
    >
      <Spinner size={size} />
      <span className={tip ? "text-sm text-text-secondary" : "sr-only"}>
        {tip ?? "Đang tải"}
      </span>
    </output>
  );

  if (children === undefined) {
    return spinning ? (
      <div className={`flex justify-center py-4 ${className}`} {...props}>
        {indicator}
      </div>
    ) : null;
  }

  return (
    <div
      className={`relative ${className}`}
      aria-busy={spinning ? "true" : undefined}
      {...props}
    >
      <div
        className={
          spinning ? "pointer-events-none select-none opacity-50" : undefined
        }
      >
        {children}
      </div>
      {spinning && (
        <div className="absolute inset-0 flex items-center justify-center">
          {indicator}
        </div>
      )}
    </div>
  );
}
