import {
  Timeline as PlatformTimeline,
  type TimelineItem,
} from "@fe-platform/ui";
import type React from "react";
import { ErrorState } from "./DataState";
import { Empty } from "./Empty";
import { Skeleton } from "./Skeleton";

export type { TimelineItem } from "@fe-platform/ui";

export interface TimelineProps {
  items: TimelineItem[];
  loading?: boolean;
  error?: React.ReactNode;
  onRetry?: () => void;
  emptyText?: React.ReactNode;
}

export function Timeline({
  items,
  loading = false,
  error,
  onRetry,
  emptyText,
}: TimelineProps) {
  if (error) return <ErrorState error={error} onRetry={onRetry} />;
  if (loading) {
    return (
      <div aria-busy="true">
        <Skeleton variant="text" lines={Math.max(3, items.length)} />
      </div>
    );
  }
  if (items.length === 0) return <Empty description={emptyText} />;
  return <PlatformTimeline items={items} />;
}
