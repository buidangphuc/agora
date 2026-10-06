import React from "react";
import { Skeleton } from "./Skeleton";

export interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  hoverable?: boolean;
  /** Replaces the content with a Skeleton while data loads; aria-busy is set. */
  loading?: boolean;
}

export function Card({
  children,
  className = "",
  hoverable = false,
  loading = false,
  ...props
}: CardProps) {
  return (
    <div
      aria-busy={loading ? "true" : undefined}
      className={`bg-surface-card text-text-primary border border-border-subtle rounded-xl shadow-preline-card overflow-hidden transition duration-200 ${
        hoverable
          ? "hover:shadow-preline-hover hover:-translate-y-0.5 hover:border-border-strong"
          : ""
      } ${className}`}
      {...props}
    >
      {loading ? (
        <div className="p-5">
          <Skeleton variant="text" lines={3} />
        </div>
      ) : (
        children
      )}
    </div>
  );
}

export function CardHeader({
  children,
  className = "",
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`px-5 py-4 border-b border-border-subtle flex items-center justify-between gap-3 ${className}`}
      {...props}
    >
      {children}
    </div>
  );
}

export function CardTitle({
  children,
  className = "",
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h3
      className={`font-semibold text-text-primary text-base leading-snug ${className}`}
      {...props}
    >
      {children}
    </h3>
  );
}

export function CardDescription({
  children,
  className = "",
  ...props
}: React.HTMLAttributes<HTMLParagraphElement>) {
  return (
    <p className={`text-xs text-text-secondary mt-0.5 ${className}`} {...props}>
      {children}
    </p>
  );
}

export function CardContent({
  children,
  className = "",
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={`p-5 ${className}`} {...props}>
      {children}
    </div>
  );
}

export function CardFooter({
  children,
  className = "",
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`px-5 py-3 bg-surface-muted border-t border-border-subtle flex items-center justify-between text-xs text-text-secondary ${className}`}
      {...props}
    >
      {children}
    </div>
  );
}
