import React from "react";
import { Card } from "./Card";
import { ErrorState } from "./DataState";
import { Skeleton } from "./Skeleton";
import { toneStyles } from "./tones";

export interface StatisticProps {
  title: React.ReactNode;
  value: React.ReactNode;
  prefix?: React.ReactNode;
  suffix?: React.ReactNode;
  trend?: {
    value: number | string;
    isUp?: boolean;
    label?: string;
  };
  /** Shows a Skeleton with the same footprint instead of the value. */
  loading?: boolean;
  /** Replaces the value with an inline error Alert. */
  error?: React.ReactNode;
  onRetry?: () => void;
  className?: string;
}

/**
 * Ant Design-style Statistic pattern.
 * Displays executive metrics, revenue counters, and KPI stats for merchant center.
 * Every row has a fixed height so loading -> value never shifts the layout.
 */
export function Statistic({
  title,
  value,
  prefix,
  suffix,
  trend,
  loading = false,
  error,
  onRetry,
  className = "",
}: StatisticProps) {
  if (error) {
    return (
      <Card className={`p-5 rounded-2xl ${className}`}>
        <span className="text-xs font-medium text-text-secondary block">
          {title}
        </span>
        <div className="mt-2">
          <ErrorState error={error} onRetry={onRetry} />
        </div>
      </Card>
    );
  }

  return (
    <Card
      aria-busy={loading ? "true" : undefined}
      className={`p-5 rounded-2xl ${className}`}
    >
      <span className="text-xs font-medium text-text-secondary block h-4">
        {title}
      </span>

      <div className="mt-2 flex h-8 items-baseline gap-1 text-2xl font-black text-text-primary tracking-tight">
        {loading ? (
          <Skeleton variant="text" lines={1} className="w-1/2" />
        ) : (
          <>
            {prefix && (
              <span className="text-lg font-bold text-text-disabled mr-1">
                {prefix}
              </span>
            )}
            <span>{value}</span>
            {suffix && (
              <span className="text-sm font-semibold text-text-secondary ml-1">
                {suffix}
              </span>
            )}
          </>
        )}
      </div>

      {trend && (
        <div className="mt-2.5 flex h-4 items-center gap-1.5 text-xs">
          {loading ? (
            <Skeleton variant="text" lines={1} className="w-1/3" />
          ) : (
            <>
              <span
                className={`font-semibold flex items-center gap-0.5 ${
                  trend.isUp ? toneStyles.success.icon : toneStyles.danger.icon
                }`}
              >
                <span>{trend.isUp ? "↑" : "↓"}</span>
                <span>{trend.value}</span>
              </span>
              {trend.label && (
                <span className="text-text-disabled">{trend.label}</span>
              )}
            </>
          )}
        </div>
      )}
    </Card>
  );
}
