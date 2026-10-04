"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { Empty } from "@/components/ui/Empty";
import { Modal } from "@/components/ui/Modal";
import { Radio } from "@/components/ui/Radio";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import type { ViewAddress } from "@/lib/gateway/addresses";
import { AddressModal } from "./AddressModal";
import { formatAddress } from "./formatAddress";

function addrHref(
  pathname: string,
  searchParams: { toString(): string },
  id: string,
): string {
  const params = new URLSearchParams(searchParams.toString());
  params.set("addr", id);
  return `${pathname}?${params.toString()}`;
}

/**
 * The Địa chỉ step body: the selected address in a Descriptions card with a
 * "Thay đổi" button, an AddressSelectorModal (Radio cards, add via the existing
 * AddressModal form) and an Empty state when the buyer has none. The choice is
 * written to `?addr=`.
 */
export function AddressSelectorModal({
  addresses,
  selectedId,
}: {
  addresses: ViewAddress[];
  selectedId?: string;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const [draftId, setDraftId] = useState<string | undefined>(selectedId);
  // Ids known when the add form was opened: a new id afterwards is the new address.
  const knownIds = useRef<Set<string> | null>(null);

  useEffect(() => {
    const known = knownIds.current;
    if (!known) return;
    const added = addresses.find((a) => !known.has(a.id));
    if (!added) return;
    knownIds.current = null;
    setDraftId(added.id);
    router.replace(addrHref(pathname, searchParams, added.id));
    toast.success("Đã thêm địa chỉ mới.");
  }, [addresses, router, pathname, searchParams, toast]);

  function startAdd() {
    knownIds.current = new Set(addresses.map((a) => a.id));
    setAdding(true);
  }

  const selected = addresses.find((a) => a.id === selectedId);

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Địa chỉ nhận hàng</CardTitle>
          {addresses.length > 0 && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setDraftId(selectedId);
                setOpen(true);
              }}
            >
              Thay đổi
            </Button>
          )}
        </CardHeader>
        <CardContent>
          {selected ? (
            <Descriptions
              column={1}
              items={[
                {
                  key: "recipient",
                  label: "Người nhận",
                  children: (
                    <span className="inline-flex flex-wrap items-center gap-2">
                      {selected.recipientName}
                      {selected.isDefault && (
                        <Tag color="primary">Mặc định</Tag>
                      )}
                    </span>
                  ),
                },
                { key: "phone", label: "Điện thoại", children: selected.phone },
                {
                  key: "address",
                  label: "Địa chỉ",
                  children: formatAddress(selected),
                },
              ]}
            />
          ) : (
            <Empty
              description="Bạn chưa có địa chỉ nhận hàng."
              action={<Button onClick={startAdd}>Thêm địa chỉ</Button>}
            />
          )}
        </CardContent>
      </Card>

      <Modal
        isOpen={open && !adding}
        onClose={() => setOpen(false)}
        title="Chọn địa chỉ nhận hàng"
        footer={
          <>
            <Button variant="outline" onClick={startAdd}>
              Thêm địa chỉ mới
            </Button>
            <Button
              disabled={!draftId}
              onClick={() => {
                if (draftId) {
                  router.replace(addrHref(pathname, searchParams, draftId));
                }
                setOpen(false);
              }}
            >
              Xác nhận
            </Button>
          </>
        }
      >
        <fieldset className="m-0 min-w-0 space-y-2 border-0 p-0">
          <legend className="sr-only">Địa chỉ</legend>
          {addresses.map((a) => {
            const isSelected = draftId === a.id;
            return (
              <div
                key={a.id}
                className={`rounded-xl border p-3 transition duration-150 ${
                  isSelected
                    ? "border-border-strong bg-surface-muted ring-2 ring-focus-ring"
                    : "border-border-subtle hover:border-border-strong"
                }`}
              >
                <Radio
                  name="shippingAddress"
                  value={a.id}
                  checked={isSelected}
                  onChange={() => setDraftId(a.id)}
                  label={
                    <span className="inline-flex flex-wrap items-center gap-2 font-semibold">
                      {a.recipientName} · {a.phone}
                      {a.isDefault && <Tag color="primary">Mặc định</Tag>}
                    </span>
                  }
                  description={formatAddress(a)}
                />
              </div>
            );
          })}
        </fieldset>
      </Modal>

      {adding && (
        <AddressModal
          onClose={() => {
            setAdding(false);
            // After a successful add the list refreshes and the effect above selects it.
            setOpen(false);
          }}
        />
      )}
    </>
  );
}
