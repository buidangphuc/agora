"use client";

import { useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent } from "@/components/ui/Card";
import { Spin } from "@/components/ui/Spin";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { formatPrice } from "@/components/ui/format";
import { usePending } from "@/features/seller/usePending";
import type { ViewMagicListing } from "@/lib/gateway/ai";
import { magicListingAction } from "./actions";

export interface MagicApply {
  title?: string;
  description?: string;
  price?: number;
}

/**
 * Magic Listing assist: asks team-ai for a suggestion from the current title
 * and category, shows it in a card and changes the form only when the seller
 * applies a field ("Áp dụng") or all of them. It never reads or writes inputs
 * itself and never overwrites anything on its own.
 */
export function MagicListingAssist({
  title,
  categoryId,
  onApply,
}: {
  title: string;
  categoryId: string;
  onApply: (values: MagicApply) => void;
}) {
  const [suggestion, setSuggestion] = useState<ViewMagicListing | null>(null);
  const [error, setError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  async function generate() {
    setError("");
    const res = await run(() => magicListingAction(title, categoryId));
    if (res.ok && res.data) {
      setSuggestion(res.data);
      return;
    }
    const message = res.ok ? "AI chưa có gợi ý cho sản phẩm này." : res.error;
    setError(message);
    toast.error(message);
  }

  const s = suggestion;
  const price = s && s.suggestedPriceMin > 0 ? s.suggestedPriceMin : undefined;

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 className="text-base font-semibold text-text-primary">
              Gợi ý bằng AI
            </h2>
            <p className="mt-0.5 text-xs text-text-secondary">
              Nhập tên sản phẩm rồi nhận gợi ý tiêu đề, mô tả và giá. Bạn chọn
              phần nào muốn áp dụng.
            </p>
          </div>
          <Button
            variant="outline"
            onClick={generate}
            isLoading={pending}
            disabled={title.trim() === ""}
          >
            Tạo gợi ý
          </Button>
        </div>

        {error && (
          <Alert
            type="error"
            description={error}
            action={
              <Button
                size="sm"
                variant="outline"
                onClick={generate}
                isLoading={pending}
              >
                Thử lại
              </Button>
            }
          />
        )}

        {pending && <Spin tip="Đang tạo gợi ý..." />}

        {s && !pending && (
          <div
            className="space-y-3 rounded-xl border border-border-subtle bg-surface-muted p-4"
            data-testid="magic-suggestion"
          >
            {s.generatedTitle && (
              <SuggestionRow
                label="Tiêu đề"
                value={s.generatedTitle}
                onApply={() => onApply({ title: s.generatedTitle })}
              />
            )}
            {s.generatedDescription && (
              <SuggestionRow
                label="Mô tả"
                value={s.generatedDescription}
                multiline
                onApply={() => onApply({ description: s.generatedDescription })}
              />
            )}
            {price !== undefined && (
              <SuggestionRow
                label="Khoảng giá gợi ý"
                value={`${formatPrice(s.suggestedPriceMin)} - ${formatPrice(
                  s.suggestedPriceMax,
                )}`}
                applyLabel="Áp dụng giá thấp nhất"
                onApply={() => onApply({ price })}
              />
            )}
            {s.highlightTags.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {s.highlightTags.map((t) => (
                  <Tag key={t}>{t}</Tag>
                ))}
              </div>
            )}
            <Button
              size="sm"
              onClick={() =>
                onApply({
                  title: s.generatedTitle || undefined,
                  description: s.generatedDescription || undefined,
                  price,
                })
              }
            >
              Áp dụng tất cả
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function SuggestionRow({
  label,
  value,
  multiline = false,
  applyLabel = "Áp dụng",
  onApply,
}: {
  label: string;
  value: string;
  multiline?: boolean;
  applyLabel?: string;
  onApply: () => void;
}) {
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
      <div className="min-w-0">
        <p className="text-xs font-medium text-text-secondary">{label}</p>
        <p
          className={`mt-0.5 text-sm text-text-primary ${
            multiline ? "whitespace-pre-line" : ""
          }`}
        >
          {value}
        </p>
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={onApply}
        aria-label={`${applyLabel}: ${label}`}
        className="shrink-0"
      >
        {applyLabel}
      </Button>
    </div>
  );
}
