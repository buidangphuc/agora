import {
  Skeleton as PlatformSkeleton,
  type SkeletonProps as PlatformSkeletonProps,
} from "@fe-platform/ui";

export type { SkeletonVariant } from "@fe-platform/ui";

export type SkeletonProps = PlatformSkeletonProps;

export function Skeleton({ label = "Đang tải", ...props }: SkeletonProps) {
  return <PlatformSkeleton label={label} {...props} />;
}
