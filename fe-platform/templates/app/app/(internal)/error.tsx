"use client";

import { ErrorPanel } from "../../components/ErrorPanel";

export default function InternalErrorBoundary({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <ErrorPanel
      title="This page could not be loaded"
      reference={error.digest}
      reset={reset}
    />
  );
}
