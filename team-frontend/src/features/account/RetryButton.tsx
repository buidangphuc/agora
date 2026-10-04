"use client";

import { useRouter } from "next/navigation";
import { useTransition } from "react";

import { Button } from "@/components/ui/Button";

/** "Thử lại" action for an inline error Alert: re-runs the route's server reads. */
export function RetryButton() {
  const router = useRouter();
  const [pending, start] = useTransition();
  return (
    <Button
      size="sm"
      variant="outline"
      isLoading={pending}
      onClick={() => start(() => router.refresh())}
    >
      Thử lại
    </Button>
  );
}
