import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { Card } from "./Card";
import { Skeleton } from "./Skeleton";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { statistic } = components;

export type StatisticTrend = {
  value: number | string;
  isUp?: boolean;
  label?: string;
};

export type StatisticProps = NativeProps<"div"> & {
  title: ReactNode;
  value: ReactNode;
  prefix?: ReactNode;
  suffix?: ReactNode;
  trend?: StatisticTrend;
  tone?: "default" | "danger";
  loading?: boolean;
  fallback?: ReactNode;
  loadingLabel?: string;
};

/** Metric card with a title, a fixed-height value row and an optional trend, so loading never shifts layout. */
export function Statistic({
  title,
  value,
  prefix,
  suffix,
  trend,
  tone = "default",
  loading = false,
  fallback,
  loadingLabel,
  ...rest
}: StatisticProps) {
  return (
    <div {...rest} aria-busy={(loading && !fallback) || undefined}>
      <Card>
        <span className={statistic.title}>{title}</span>
        {fallback ? (
          <div className={statistic.fallback}>{fallback}</div>
        ) : (
          <>
            <div
              className={cx(
                statistic.value,
                tone === "danger" && statistic.danger,
              )}
            >
              {loading ? (
                <div className={statistic.valueSkeleton}>
                  <Skeleton lines={1} label={loadingLabel} />
                </div>
              ) : (
                <>
                  {prefix && <span className={statistic.prefix}>{prefix}</span>}
                  <span>{value}</span>
                  {suffix && <span className={statistic.suffix}>{suffix}</span>}
                </>
              )}
            </div>
            {trend && (
              <div className={statistic.trend}>
                {loading ? (
                  <div className={statistic.trendSkeleton}>
                    <Skeleton lines={1} label={loadingLabel} />
                  </div>
                ) : (
                  <>
                    <span
                      className={trend.isUp ? statistic.up : statistic.down}
                    >
                      <span>{trend.isUp ? "↑" : "↓"}</span>
                      <span>{trend.value}</span>
                    </span>
                    {trend.label && (
                      <span className={statistic.trendLabel}>
                        {trend.label}
                      </span>
                    )}
                  </>
                )}
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  );
}
