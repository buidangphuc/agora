"use client";

import Link from "next/link";
import {
  type ReactNode,
  createContext,
  useContext,
  useMemo,
  useState,
} from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { OrderSummary } from "@/features/order/OrderSummary";
import { linkButtonPrimary } from "./linkButton";
import { resolveSelection, selectionHref } from "./selection";

export interface SelectionLine {
  id: string;
  sellerId: string;
  unitPrice: number;
  quantity: number;
}

interface SelectionState {
  selected: Set<string>;
  lines: SelectionLine[];
  toggleLine: (id: string) => void;
  toggleShop: (sellerId: string) => void;
}

const SelectionContext = createContext<SelectionState | null>(null);

export function CartSelection({
  lines,
  initialItems,
  checkoutEnabled,
  discount,
  voucherCode,
  children,
}: {
  lines: SelectionLine[];
  initialItems?: string;
  checkoutEnabled: boolean;
  discount: number;
  voucherCode?: string;
  children: ReactNode;
}) {
  const [selected, setSelected] = useState<Set<string>>(() => {
    const ids = lines.map((l) => l.id);
    const restored = resolveSelection(ids, initialItems);
    return new Set(restored.length > 0 ? restored : ids);
  });

  const state = useMemo<SelectionState>(
    () => ({
      selected,
      lines,
      toggleLine: (id) =>
        setSelected((prev) => {
          const next = new Set(prev);
          if (!next.delete(id)) next.add(id);
          return next;
        }),
      toggleShop: (sellerId) =>
        setSelected((prev) => {
          const shopIds = lines
            .filter((l) => l.sellerId === sellerId)
            .map((l) => l.id);
          const allOn = shopIds.every((id) => prev.has(id));
          const next = new Set(prev);
          for (const id of shopIds) {
            if (allOn) next.delete(id);
            else next.add(id);
          }
          return next;
        }),
    }),
    [selected, lines],
  );

  const chosen = lines.filter((l) => selected.has(l.id));
  const subtotal = chosen.reduce((a, l) => a + l.unitPrice * l.quantity, 0);
  const partial = chosen.length < lines.length;
  const canBuy = checkoutEnabled && chosen.length > 0;
  const href = selectionHref(chosen.map((l) => l.id));

  const notices = (
    <>
      {checkoutEnabled ? undefined : (
        <Alert
          type="warning"
          description="Thanh toán tạm thời không khả dụng. Vui lòng thử lại sau."
        />
      )}
      {partial && voucherCode ? (
        <p className="text-sm text-text-secondary">
          Giảm giá được tính ở bước thanh toán
        </p>
      ) : undefined}
    </>
  );

  return (
    <SelectionContext.Provider value={state}>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">{children}</div>
        <div>
          <OrderSummary
            itemCount={chosen.length}
            subtotal={subtotal}
            discount={partial ? 0 : discount}
            hideDiscount={partial}
            voucherCode={partial ? undefined : voucherCode}
            shipping={null}
            notice={notices}
            action={
              canBuy ? (
                <Link href={href} className={`${linkButtonPrimary} w-full`}>
                  Mua hàng
                </Link>
              ) : (
                <Button size="lg" disabled fullWidth>
                  Mua hàng
                </Button>
              )
            }
            mobileAction={
              canBuy ? (
                <Link href={href} className={linkButtonPrimary}>
                  Mua hàng
                </Link>
              ) : (
                <Button size="lg" disabled>
                  Mua hàng
                </Button>
              )
            }
          />
        </div>
      </div>
    </SelectionContext.Provider>
  );
}

export function LineCheckbox({ id, title }: { id: string; title: string }) {
  const ctx = useContext(SelectionContext);
  if (!ctx) return null;
  return (
    <Checkbox
      data-testid={`cart-line-checkbox-${id}`}
      label={`Chọn ${title}`}
      labelHidden
      checked={ctx.selected.has(id)}
      onCheckedChange={() => ctx.toggleLine(id)}
    />
  );
}

export function ShopCheckbox({
  sellerId,
  name,
}: {
  sellerId: string;
  name: string;
}) {
  const ctx = useContext(SelectionContext);
  const shopLines = ctx?.lines.filter((l) => l.sellerId === sellerId) ?? [];
  const on = shopLines.filter((l) => ctx?.selected.has(l.id)).length;
  const all = shopLines.length > 0 && on === shopLines.length;
  const some = on > 0 && !all;
  if (!ctx) return null;
  return (
    <Checkbox
      data-testid={`cart-shop-checkbox-${sellerId}`}
      label={`Chọn tất cả sản phẩm của ${name}`}
      labelHidden
      checked={all ? true : some ? "indeterminate" : false}
      onCheckedChange={() => ctx.toggleShop(sellerId)}
    />
  );
}
