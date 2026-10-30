"use client";

import { Button } from "@/components/ui/Button";

/** Prints the page; the print rules hide everything but the packing slip. */
export function PrintButton({ label = "In phiếu" }: { label?: string }) {
  return (
    <Button variant="outline" size="md" onClick={() => window.print()}>
      {label}
    </Button>
  );
}
