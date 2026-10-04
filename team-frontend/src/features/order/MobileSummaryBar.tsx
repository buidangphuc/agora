"use client";

import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Drawer } from "@/components/ui/Drawer";
import { PriceTag } from "@/components/ui/PriceTag";

/**
 * Mobile-only sticky bottom bar: the total, a "Chi tiết" button opening a Drawer
 * with the full breakdown (`children`, rendered on the server) and the primary
 * action. Hidden from lg up, where the summary card is shown instead.
 */
export function MobileSummaryBar({
  total,
  action,
  children,
}: {
  total: number;
  action: ReactNode;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <div
        data-testid="mobile-summary-bar"
        className="fixed inset-x-0 bottom-0 z-40 flex items-center gap-3 border-t border-border-subtle bg-surface-card px-4 py-3 lg:hidden"
      >
        <div className="min-w-0 flex-1">
          <p className="text-xs text-text-secondary">Tổng cộng</p>
          <PriceTag price={total} size="lg" />
        </div>
        <Button variant="outline" size="md" onClick={() => setOpen(true)}>
          Chi tiết
        </Button>
        {action}
      </div>
      <Drawer
        isOpen={open}
        onClose={() => setOpen(false)}
        title="Chi tiết đơn hàng"
        placement="right"
      >
        {children}
      </Drawer>
    </>
  );
}
