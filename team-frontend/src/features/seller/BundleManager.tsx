"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Checkbox } from "@/components/ui/Checkbox";
import { FormItem } from "@/components/ui/FormItem";
import { Input } from "@/components/ui/Input";
import { PriceTag } from "@/components/ui/PriceTag";
import { Table, type TableColumn } from "@/components/ui/Table";
import { useToast } from "@/components/ui/ToastProvider";
import type { ViewBundle } from "@/lib/gateway/listings";
import { createBundleAction } from "./actions";
import { usePending } from "./usePending";

interface SellerListingOption {
  id: string;
  title: string;
}

type Field = "title" | "items" | "price";

/** Client-side rules, the same texts as createBundleAction. */
export function validateBundle(
  title: string,
  selected: number,
  price: number,
): Partial<Record<Field, string>> {
  const errors: Partial<Record<Field, string>> = {};
  if (!title.trim()) errors.title = "Nhập tên combo.";
  if (selected < 2) errors.items = "Chọn ít nhất 2 sản phẩm cho combo.";
  if (!(price > 0)) errors.price = "Giá combo phải lớn hơn 0.";
  return errors;
}

const bundleColumns: TableColumn<ViewBundle>[] = [
  { key: "title", title: "Combo", dataIndex: "title" },
  {
    key: "count",
    title: "Số sản phẩm",
    align: "right",
    render: (b) => b.listingIds.length,
  },
  {
    key: "price",
    title: "Giá combo",
    align: "right",
    render: (b) => <PriceTag price={b.bundlePrice} size="md" />,
  },
  { key: "createdAt", title: "Ngày tạo", dataIndex: "createdAt" },
];

/**
 * Bundle manager: pick >= 2 of the seller's listings, set a bundle price and
 * create a combo. The list comes from the server (`bundles`), so it refreshes
 * through revalidatePath after a successful create.
 */
export function BundleManager({
  listings,
  bundles,
}: {
  listings: SellerListingOption[];
  bundles: ViewBundle[];
}) {
  const [title, setTitle] = useState("");
  const [price, setPrice] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const errors = validateBundle(title, selected.length, Number(price));
  const shown = (field: Field) => (submitted ? errors[field] : undefined);

  function toggle(id: string) {
    setSelected((prev) =>
      prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id],
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitted(true);
    setError("");
    if (Object.keys(errors).length > 0) return;
    const res = await run(() =>
      createBundleAction(title, selected, Number(price)),
    );
    if (res.ok) {
      setTitle("");
      setPrice("");
      setSelected([]);
      setSubmitted(false);
      toast.success("Đã tạo combo");
    } else {
      setError(res.error);
      toast.error(res.error);
    }
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Tạo combo mới</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} noValidate className="space-y-4">
            <fieldset disabled={pending} className="m-0 space-y-4 border-0 p-0">
              <FormItem
                label="Tên combo"
                required
                status={shown("title") ? "error" : undefined}
                help={shown("title")}
              >
                <Input
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              </FormItem>
              <FormItem
                label="Giá combo (₫)"
                required
                status={shown("price") ? "error" : undefined}
                help={shown("price")}
              >
                <Input
                  type="number"
                  min={0}
                  value={price}
                  onChange={(e) => setPrice(e.target.value)}
                />
              </FormItem>
              <FormItem
                group
                label={`Chọn sản phẩm (${selected.length} đã chọn)`}
                required
                status={shown("items") ? "error" : undefined}
                help={shown("items")}
              >
                {listings.length === 0 ? (
                  <p className="text-sm text-text-secondary">
                    Shop chưa có sản phẩm nào.
                  </p>
                ) : (
                  <ul className="max-h-56 space-y-2 overflow-y-auto rounded-xl border border-border-subtle p-3">
                    {listings.map((l) => (
                      <li key={l.id}>
                        <Checkbox
                          label={l.title}
                          checked={selected.includes(l.id)}
                          onChange={() => toggle(l.id)}
                        />
                      </li>
                    ))}
                  </ul>
                )}
              </FormItem>
            </fieldset>
            {error && <Alert type="error" description={error} />}
            <Button type="submit" isLoading={pending}>
              Tạo combo
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Combo hiện có ({bundles.length})</CardTitle>
        </CardHeader>
        <CardContent>
          <Table
            caption="Combo hiện có"
            columns={bundleColumns}
            dataSource={bundles}
            rowKey="id"
            emptyText="Chưa có combo nào"
          />
        </CardContent>
      </Card>
    </div>
  );
}
