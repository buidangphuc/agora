import type { ReactNode } from "react";

export function SectionHeading({
  id,
  title,
  description,
  action,
}: {
  id: string;
  title: ReactNode;
  description?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
      <div className="min-w-0 space-y-1">
        <h2
          id={id}
          className="text-xl font-semibold tracking-tight text-text-primary sm:text-2xl"
        >
          {title}
        </h2>
        {description ? (
          <p className="text-sm text-text-secondary">{description}</p>
        ) : null}
      </div>
      {action ? <div className="shrink-0">{action}</div> : null}
    </div>
  );
}
