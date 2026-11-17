"use client";

import { useState } from "react";

import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/ToastProvider";
import { bulkShipOrdersAction } from "@/features/order/actions";
import type { ViewOrder } from "@/lib/gateway/orders";
import { useBulkShipSelection } from "./BulkShipSelection";
import { usePending } from "./usePending";

interface Row {
  orderId: string;
  ok: boolean;
  error?: string;
}

export function BulkShipModal({
  orders,
  isOpen,
  onClose,
}: {
  orders: ViewOrder[];
  isOpen: boolean;
  onClose: () => void;
}) {
  const { selected, setSelected } = useBulkShipSelection();
  const [tracking, setTracking] = useState<Record<string, string>>({});
  const [results, setResults] = useState<Row[] | null>(null);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const chosen = orders.filter((o) => selected.includes(o.id));

  async function submit() {
    setError("");
    const payload = chosen.map((o) => {
      const t = (tracking[o.id] ?? "").trim();
      return t ? { orderId: o.id, trackingNumber: t } : { orderId: o.id };
    });
    const res = await run(() => bulkShipOrdersAction(payload));
    if (!res.ok) {
      setError(res.error);
      toast.error(res.error);
      return;
    }
    const rows = (res.data?.results ?? []) as Row[];
    setResults(rows);
    setSelected(rows.filter((r) => !r.ok).map((r) => r.orderId));
    const failed = rows.filter((r) => !r.ok).length;
    if (failed === 0) toast.success("Đã bàn giao vận chuyển");
    else toast.error(`${failed} đơn chưa gửi được`);
    setTracking({});
    onClose();
  }

  function close() {
    if (pending) return;
    setError("");
    setTracking({});
    onClose();
  }

  return (
    <>
      {results && (
        <section aria-label="Kết quả gửi hàng loạt" className="w-full">
          <ul className="space-y-1">
            {results.map((r) => (
              <li key={r.orderId} className="text-sm">
                <span className="font-medium">#{r.orderId.slice(0, 8)}</span>{" "}
                {r.ok ? (
                  <span>Đã gửi</span>
                ) : (
                  <span className="text-danger">{r.error}</span>
                )}
              </li>
            ))}
          </ul>
          <Button size="xs" variant="ghost" onClick={() => setResults(null)}>
            Ẩn kết quả
          </Button>
        </section>
      )}
      <Modal
        isOpen={isOpen}
        onClose={close}
        size="md"
        title="Gửi hàng loạt"
        description="Các đơn sẽ chuyển sang trạng thái Đang giao."
        footer={
          <>
            <Button variant="outline" onClick={close} disabled={pending}>
              Huỷ
            </Button>
            <Button
              onClick={submit}
              isLoading={pending}
              disabled={chosen.length === 0}
            >
              Xác nhận gửi hàng loạt
            </Button>
          </>
        }
      >
        <div className="space-y-3">
          {chosen.map((o) => (
            <Input
              key={o.id}
              label={`Mã vận đơn #${o.id.slice(0, 8)}`}
              value={tracking[o.id] ?? ""}
              onChange={(e) =>
                setTracking((cur) => ({ ...cur, [o.id]: e.target.value }))
              }
              disabled={pending}
            />
          ))}
          {error && <p className="text-sm text-danger">{error}</p>}
        </div>
      </Modal>
    </>
  );
}
