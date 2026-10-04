"use client";

import Link from "next/link";
import { type FormEvent, useState } from "react";

import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { useToast } from "@/components/ui/ToastProvider";
import { usePendingAction } from "@/features/account/usePendingAction";
import type { ViewCollection } from "@/lib/gateway/engagement";
import { createCollectionResultAction } from "./actions";

/**
 * Wishlist collections panel on the favorites page: create a named collection
 * and see existing ones. Item add/remove happens from the listing page via
 * AddToCollectionButton.
 */
export function CollectionsManager({
  initialCollections,
}: {
  initialCollections: ViewCollection[];
}) {
  const [collections, setCollections] =
    useState<ViewCollection[]>(initialCollections);
  const [name, setName] = useState("");
  const { pending, run } = usePendingAction();
  const toast = useToast();

  function handleCreate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    void run(async () => {
      const res = await createCollectionResultAction(trimmed);
      if (res.ok && res.data) {
        const created = res.data;
        setCollections((prev) => [created, ...prev]);
        setName("");
        toast.success(`Đã tạo bộ sưu tập "${trimmed}".`);
      } else if (!res.ok) {
        toast.error(res.error);
      }
    });
  }

  return (
    <Card>
      <CardHeader className="flex-col items-start justify-start gap-0">
        <h2 className="text-base font-semibold text-text-primary">
          Bộ sưu tập của tôi
        </h2>
        <p className="mt-0.5 text-xs text-text-secondary">
          Nhóm các sản phẩm yêu thích thành danh sách riêng để dễ theo dõi.
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        <form
          aria-label="Tạo bộ sưu tập"
          onSubmit={handleCreate}
          className="flex flex-col gap-2 sm:flex-row"
        >
          <div className="flex-1">
            <Input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Tên bộ sưu tập mới…"
              aria-label="Tên bộ sưu tập"
            />
          </div>
          <Button
            type="submit"
            isLoading={pending}
            disabled={!name.trim()}
            className="min-h-10 w-full sm:w-auto"
          >
            Tạo mới
          </Button>
        </form>

        {collections.length === 0 ? (
          <p className="py-2 text-sm text-text-secondary">
            Bạn chưa có bộ sưu tập nào. Tạo một bộ sưu tập để bắt đầu nhé.
          </p>
        ) : (
          <ul className="divide-y divide-border-subtle">
            {collections.map((c) => (
              <li
                key={c.id}
                data-testid="collection-row"
                data-name={c.name}
                className="flex items-center justify-between gap-3 py-3"
              >
                <Link
                  href={`/favorites?collection=${c.id}`}
                  className="min-w-0 truncate text-sm font-medium text-text-primary hover:text-action-primary"
                >
                  {c.name}
                </Link>
                <Badge variant="neutral" className="shrink-0">
                  {c.itemCount} sản phẩm
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
