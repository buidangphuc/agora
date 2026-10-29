"use client";

import { ErrorPanel } from "../components/ErrorPanel";

export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorPanel
      title="Something went wrong"
      reference={error.digest}
      reset={reset}
    />
  );
}
