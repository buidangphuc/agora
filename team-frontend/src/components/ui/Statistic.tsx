import {
  Statistic as PlatformStatistic,
  type StatisticProps as PlatformStatisticProps,
} from "@fe-platform/ui";
import type React from "react";
import { ErrorState } from "./DataState";

export interface StatisticProps
  extends Omit<PlatformStatisticProps, "fallback" | "loadingLabel"> {
  error?: React.ReactNode;
  onRetry?: () => void;
}

export function Statistic({ error, onRetry, ...props }: StatisticProps) {
  return (
    <PlatformStatistic
      {...props}
      fallback={
        error ? <ErrorState error={error} onRetry={onRetry} /> : undefined
      }
      loadingLabel="Đang tải"
    />
  );
}
