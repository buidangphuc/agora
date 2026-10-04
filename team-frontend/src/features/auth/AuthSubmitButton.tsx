"use client";

import { useFormStatus } from "react-dom";

import { Button } from "@/components/ui/Button";

/**
 * Submit button of the auth forms. `useFormStatus` makes it pending (spinner,
 * aria-busy, inert, width kept) for the whole server-action round trip, so a
 * second click cannot send a second request.
 */
export function AuthSubmitButton({ children }: { children: string }) {
  const { pending } = useFormStatus();
  return (
    <Button type="submit" size="lg" isLoading={pending} className="w-full">
      {children}
    </Button>
  );
}
