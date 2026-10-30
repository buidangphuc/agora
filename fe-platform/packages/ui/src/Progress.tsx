import { components } from "@fe-platform/tokens";
import { cx } from "./cx";
import type { NativeProps } from "./types";

const { progress } = components;

export type ProgressProps = NativeProps<"div"> & {
  label: string;
  percent: number;
  status?: keyof typeof progress.status;
  showInfo?: boolean;
  size?: keyof typeof progress.size;
};

/** Line progress bar backed by a native progress element, with an optional percentage. */
export function Progress({
  label,
  percent,
  status = "normal",
  showInfo = true,
  size = "md",
  ...rest
}: ProgressProps) {
  const value = Math.min(100, Math.max(0, Math.round(percent)));
  return (
    <div {...rest} className={progress.root}>
      <progress
        className="sr-only"
        aria-label={label}
        aria-valuenow={value}
        aria-valuemin={0}
        aria-valuemax={100}
        value={value}
        max={100}
      />
      <div
        aria-hidden="true"
        className={cx(progress.track, progress.size[size])}
      >
        <div
          className={cx(progress.fill, progress.status[status])}
          style={{ width: `${value}%` }}
        />
      </div>
      {showInfo && <span className={progress.info}>{value}%</span>}
    </div>
  );
}
