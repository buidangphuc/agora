"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import type { ViewOrder } from "@/lib/gateway/orders";
import { BulkShipModal } from "./BulkShipModal";
import { useBulkShipSelection } from "./BulkShipSelection";
import { BULK_SHIP_MAX } from "./bulkShip";
import { isToShip } from "./status";

export function BulkShipBar({ orders }: { orders: ViewOrder[] }) {
  const { selected, setSelected, error } = useBulkShipSelection();
  const [open, setOpen] = useState(false);
  const shippable = orders.filter(isToShip).map((o) => o.id);
  const allSelected =
    shippable.length > 0 && shippable.every((id) => selected.includes(id));
  const empty = selected.length === 0;

  function toggleAll() {
    setSelected(allSelected ? [] : shippable.slice(0, BULK_SHIP_MAX));
  }

  function print() {
    window.open(
      `/seller/orders/print?ids=${encodeURIComponent(selected.join(","))}`,
      "_blank",
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-3">
      <label className="flex items-center gap-2 text-sm">
        <input
          type="checkbox"
          aria-label="Chọn tất cả đơn có thể gửi"
          checked={allSelected}
          disabled={shippable.length === 0}
          onChange={toggleAll}
        />
        Chọn tất cả
      </label>
      <span className="text-sm text-text-secondary">
        Đã chọn {selected.length} đơn
      </span>
      <Button
        size="sm"
        variant="outline"
        disabled={empty}
        onClick={() => setOpen(true)}
      >
        Gửi hàng loạt
      </Button>
      <Button size="sm" variant="outline" disabled={empty} onClick={print}>
        In phiếu hàng loạt
      </Button>
      {error && (
        <span role="alert" className="text-sm text-danger">
          {error}
        </span>
      )}
      <BulkShipModal
        orders={orders}
        isOpen={open}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}
