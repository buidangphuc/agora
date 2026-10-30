import type { ReactNode } from "react";

export function Page({
  title,
  description,
  width = "wide",
  children,
}: {
  title: string;
  description?: string;
  width?: "wide" | "narrow";
  children?: ReactNode;
}) {
  return (
    <main
      className={`mx-auto flex w-full flex-col gap-8 px-4 py-8 md:px-6 md:py-12 ${width === "narrow" ? "max-w-lg" : "max-w-5xl"}`}
    >
      <header className="flex flex-col gap-2">
        <h1 className="text-3xl font-semibold tracking-tight md:text-4xl">
          {title}
        </h1>
        {description ? (
          <p className="text-base text-text-secondary">{description}</p>
        ) : null}
      </header>
      {children}
    </main>
  );
}
