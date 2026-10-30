import type { ReactNode } from "react";

import { Card } from "@/components/ui/Card";

export function AuthPanel({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <section className="mx-auto flex w-full max-w-md flex-col gap-6 py-6 md:gap-8 md:py-12">
      <header className="flex flex-col gap-2 text-center">
        <h1 className="text-2xl font-semibold tracking-tight text-text-primary md:text-3xl">
          {title}
        </h1>
        <p className="text-base text-text-secondary">{description}</p>
      </header>
      <Card padded>{children}</Card>
    </section>
  );
}

export function AuthSwitch({
  prompt,
  children,
}: {
  prompt: string;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 border-t border-border-subtle pt-6 text-center">
      <p className="text-sm text-text-secondary">{prompt}</p>
      {children}
    </div>
  );
}
