"use client";

import { Button } from "@fe-platform/ui";

export function ErrorPanel({
  title,
  reference,
  reset,
}: {
  title: string;
  reference?: string;
  reset: () => void;
}) {
  return (
    <main className="flex flex-col gap-4 p-6">
      <div role="alert" className="flex flex-col gap-2">
        <h1 className="text-2xl">{title}</h1>
        {reference ? (
          <p className="text-text-secondary">Reference: {reference}</p>
        ) : null}
      </div>
      <div>
        <Button onClick={reset}>Try again</Button>
      </div>
    </main>
  );
}
