"use client";

import { Button, Card } from "@fe-platform/ui";

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
    <main className="mx-auto flex w-full max-w-lg flex-col gap-8 px-4 py-8 md:px-6 md:py-12">
      <Card>
        <div className="flex flex-col gap-6">
          <div role="alert" className="flex flex-col gap-2">
            <h1 className="text-2xl font-semibold">{title}</h1>
            {reference ? (
              <p className="text-text-secondary">Reference: {reference}</p>
            ) : null}
          </div>
          <div>
            <Button onClick={reset}>Try again</Button>
          </div>
        </div>
      </Card>
    </main>
  );
}
