import { components } from "@fe-platform/tokens";
import type { ReactNode } from "react";
import { cx } from "./cx";

const { timeline } = components;

export type TimelineTone = keyof typeof timeline.tone;

export type TimelineItem = {
  key: string;
  title: ReactNode;
  description?: ReactNode;
  time?: ReactNode;
  tone?: TimelineTone;
  current?: boolean;
  testId?: string;
};

export type TimelineProps = {
  items: TimelineItem[];
};

/** Vertical event history (shipment checkpoints, order steps) with a toned dot per event; the latest event carries `aria-current="step"`. */
export function Timeline({ items }: TimelineProps) {
  return (
    <ol className={timeline.list}>
      {items.map((item, idx) => {
        const tone = item.tone ?? "neutral";
        return (
          <li
            key={item.key}
            aria-current={item.current ? "step" : undefined}
            data-testid={item.testId}
            className={timeline.item}
          >
            {idx < items.length - 1 && (
              <div aria-hidden="true" className={timeline.line} />
            )}
            <div className={timeline.dotWrap}>
              <span
                aria-hidden="true"
                className={cx(timeline.dot, timeline.tone[tone])}
              />
            </div>
            <div className={timeline.body}>
              <p
                className={cx(
                  timeline.title,
                  item.current
                    ? timeline.titleTone[tone]
                    : timeline.titleDefault,
                )}
              >
                {item.title}
                {item.time && (
                  <span className={timeline.time}>{item.time}</span>
                )}
              </p>
              {item.description && (
                <p className={timeline.description}>{item.description}</p>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
