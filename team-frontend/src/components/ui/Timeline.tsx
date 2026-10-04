import React from "react";
import { ErrorState } from "./DataState";
import { Empty } from "./Empty";
import { Skeleton } from "./Skeleton";
import { type Tone, toneStyles } from "./tones";

export interface TimelineItem {
  key: string;
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Timestamp or relative time shown beside the title. */
  time?: React.ReactNode;
  /** Dot colour; defaults to neutral. */
  tone?: Tone;
  /** Marks the latest event (`aria-current="step"`). */
  current?: boolean;
  /** `data-testid` of the item, for e2e hooks. */
  testId?: string;
}

export interface TimelineProps {
  items: TimelineItem[];
  loading?: boolean;
  error?: React.ReactNode;
  onRetry?: () => void;
  emptyText?: React.ReactNode;
  className?: string;
}

// Tier 3: dot fills per tone (aliases where they exist).
const dotFill: Record<Tone, string> = {
  neutral: "bg-border-strong",
  primary: "bg-action-primary",
  info: "bg-text-secondary",
  success: "bg-success",
  warning: "bg-promo",
  danger: "bg-danger",
};

/**
 * Ant Design `Timeline`: a vertical event list (shipment history, saga
 * checkpoints). Server-compatible, with loading / empty / error states.
 */
export function Timeline({
  items,
  loading = false,
  error,
  onRetry,
  emptyText,
  className = "",
}: TimelineProps) {
  if (error)
    return <ErrorState error={error} onRetry={onRetry} className={className} />;
  if (loading) {
    return (
      <div aria-busy="true" className={className}>
        <Skeleton variant="text" lines={Math.max(3, items.length)} />
      </div>
    );
  }
  if (items.length === 0) {
    return <Empty description={emptyText} className={className} />;
  }
  return (
    <ol className={`space-y-4 ${className}`}>
      {items.map((item, idx) => {
        const isLast = idx === items.length - 1;
        const tone = item.tone ?? "neutral";
        return (
          <li
            key={item.key}
            aria-current={item.current ? "step" : undefined}
            data-testid={item.testId}
            className="relative flex gap-4"
          >
            {!isLast && (
              <div
                aria-hidden="true"
                className="absolute left-2.5 top-5 -bottom-2 w-0.5 bg-border-subtle"
              />
            )}
            <div className="relative flex h-5 w-5 shrink-0 items-center justify-center">
              <span
                aria-hidden="true"
                className={`h-2.5 w-2.5 rounded-full ring-4 ring-surface-card ${dotFill[tone]}`}
              />
            </div>
            <div className="min-w-0 pb-2 pt-0.5">
              <p
                className={`text-xs font-semibold ${
                  item.current ? toneStyles[tone].icon : "text-text-primary"
                }`}
              >
                {item.title}
                {item.time && (
                  <span className="ml-2 font-normal text-text-disabled">
                    {item.time}
                  </span>
                )}
              </p>
              {item.description && (
                <p className="mt-0.5 text-xs text-text-secondary">
                  {item.description}
                </p>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
