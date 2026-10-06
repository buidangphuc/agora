"use client";

import { useSearchParams } from "next/navigation";
import { useState } from "react";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Drawer } from "@/components/ui/Drawer";

/**
 * Mobile filter container (below 1024px): a "Bộ lọc" button with the active
 * count that opens the same filters in a Drawer, with a sticky "Áp dụng" that
 * submits the price form (`formId`). The drawer closes when the URL changes.
 */
export function FilterDrawer({
  activeCount,
  formId,
  children,
}: {
  activeCount: number;
  formId: string;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const search = useSearchParams().toString();

  // A new URL (a filter was picked) closes the drawer.
  const [seen, setSeen] = useState(search);
  if (seen !== search) {
    setSeen(search);
    setOpen(false);
  }

  return (
    <div className="lg:hidden">
      <Button
        variant="outline"
        size="md"
        onClick={() => setOpen(true)}
        aria-haspopup="dialog"
        className="w-full"
      >
        <span className="inline-flex items-center gap-2">
          Bộ lọc
          {activeCount > 0 && (
            <Badge variant="primary" size="sm" pill>
              {activeCount}
            </Badge>
          )}
        </span>
      </Button>
      <Drawer
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Bộ lọc"
        footer={
          <Button type="submit" form={formId} variant="primary" size="md">
            Áp dụng
          </Button>
        }
      >
        {children}
      </Drawer>
    </div>
  );
}
