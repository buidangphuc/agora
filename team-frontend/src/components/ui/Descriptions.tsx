import {
  Descriptions as PlatformDescriptions,
  type DescriptionsProps as PlatformDescriptionsProps,
} from "@fe-platform/ui";
import type React from "react";
import { ErrorState } from "./DataState";
import { Empty } from "./Empty";

export type { DescriptionItem } from "@fe-platform/ui";

export interface DescriptionsProps
  extends Omit<
    PlatformDescriptionsProps,
    "fallback" | "emptyText" | "loadingLabel"
  > {
  error?: React.ReactNode;
  onRetry?: () => void;
  emptyText?: React.ReactNode;
}

export function Descriptions({
  error,
  onRetry,
  emptyText,
  items,
  loading = false,
  ...props
}: DescriptionsProps) {
  let fallback: React.ReactNode;
  if (error) {
    fallback = <ErrorState error={error} onRetry={onRetry} />;
  } else if (!loading && items.length === 0) {
    fallback = <Empty description={emptyText} />;
  }
  return (
    <PlatformDescriptions
      {...props}
      items={items}
      loading={loading}
      fallback={fallback}
      loadingLabel="Đang tải"
    />
  );
}
