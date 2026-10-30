import {
  Progress as PlatformProgress,
  type ProgressProps as PlatformProgressProps,
} from "@fe-platform/ui";

export interface ProgressProps extends Omit<PlatformProgressProps, "label"> {
  label?: string;
}

export function Progress({ label = "Tiến độ", ...props }: ProgressProps) {
  return <PlatformProgress label={label} {...props} />;
}
