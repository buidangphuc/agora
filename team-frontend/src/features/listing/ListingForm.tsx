"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Image } from "@/components/ui/Image";
import { Input } from "@/components/ui/Input";
import { RadioGroup } from "@/components/ui/Radio";
import { Result } from "@/components/ui/Result";
import { Select } from "@/components/ui/Select";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { LinkButton } from "@/features/seller/LinkButton";
import { usePending } from "@/features/seller/usePending";
import { getImageUrl } from "@/lib/media";
import { MagicListingAssist } from "./MagicListingAssist";
import {
  type SellState,
  getUploadUrlAction,
  saveListingAction,
} from "./actions";
import {
  type FieldErrors,
  type SellField,
  validateListingFields,
} from "./validation";

const initialSellState: SellState = { ok: false, message: "" };

export interface FormVariant {
  id?: string;
  name: string;
  sku?: string;
  price?: number;
  stock?: number;
}

export interface ListingDefaults {
  id?: string;
  title?: string;
  description?: string;
  price?: number;
  currency?: string;
  status?: string;
  imageKeys?: string[];
  categoryId?: string;
  stock?: number;
  variants?: FormVariant[];
}

export interface CategoryOption {
  id: string;
  name: string;
  iconUrl?: string;
}

const STATUS_OPTIONS = [
  {
    value: "published",
    label: "Đang bán",
    description: "Hiển thị công khai cho người mua",
  },
  {
    value: "draft",
    label: "Lưu bản nháp",
    description: "Chỉ bạn nhìn thấy sản phẩm này",
  },
];

// Native textarea on the same tokens as Input (no Textarea in the core set yet).
const textareaClass =
  "block w-full rounded-lg border border-border-subtle bg-surface-card px-3.5 py-2 text-sm text-text-primary shadow-2xs transition duration-150 placeholder:text-text-disabled focus-visible:border-action-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring aria-invalid:border-danger";

interface VariantRow {
  key: string;
  value: FormVariant;
}

/** Images above this index are below the first screen: lazy loaded. */
const EAGER_IMAGES = 6;

function toNumber(value: string): number {
  return value.trim() === "" ? Number.NaN : Number(value);
}

function Section({
  title,
  description,
  children,
}: { title: string; description?: string; children: React.ReactNode }) {
  return (
    <Card>
      <CardHeader className="flex-col items-start gap-0.5">
        <CardTitle>{title}</CardTitle>
        {description && (
          <p className="text-xs text-text-secondary">{description}</p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">{children}</CardContent>
    </Card>
  );
}

/**
 * Listing studio (Ant Pro Basic/Advanced Form): Card sections of FormItems over
 * controlled state. Required fields validate on blur and submit and again from
 * the server SellState; submit is pending and non-repeatable. The hidden
 * `id`, `imageKeys` and `variants` inputs and saveListingAction are unchanged.
 */
export function ListingForm({
  listingId,
  defaults = {},
  categories = [],
  submitLabel = "Lưu & Hiển Thị Bán",
}: {
  listingId?: string;
  defaults?: ListingDefaults;
  categories?: CategoryOption[];
  submitLabel?: string;
}) {
  const [title, setTitle] = useState(defaults.title ?? "");
  const [categoryId, setCategoryId] = useState(defaults.categoryId ?? "");
  const [description, setDescription] = useState(defaults.description ?? "");
  const [price, setPrice] = useState(
    defaults.price !== undefined ? String(defaults.price) : "",
  );
  const [stock, setStock] = useState(
    defaults.stock !== undefined ? String(defaults.stock) : "",
  );
  const [currency, setCurrency] = useState(defaults.currency ?? "VND");
  const [status, setStatus] = useState(defaults.status ?? "published");
  const [imageKeys, setImageKeys] = useState<string[]>(
    Array.isArray(defaults.imageKeys) ? defaults.imageKeys : [],
  );
  // Rows carry a client key: a variant has no stable identity until saved.
  const nextRowKey = useRef(0);
  const newRowKey = () => `variant-${nextRowKey.current++}`;
  const [rows, setRows] = useState<VariantRow[]>(() =>
    (Array.isArray(defaults.variants) ? defaults.variants : []).map((v) => ({
      key: v.id ?? newRowKey(),
      value: v,
    })),
  );
  const variants = rows.map((r) => r.value);
  const [touched, setTouched] = useState<Partial<Record<SellField, boolean>>>(
    {},
  );
  const [edited, setEdited] = useState<Partial<Record<SellField, boolean>>>({});
  const [state, setState] = useState<SellState>(initialSellState);
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const { pending, run } = usePending();
  const toast = useToast();

  const id = listingId ?? defaults.id ?? "";
  const clientErrors = validateListingFields({
    title,
    price: toNumber(price),
    stock: toNumber(stock),
    categoryId,
  });

  /** Inline error of a field: client rule once touched, else the server's. */
  function errorOf(field: SellField): string | undefined {
    if (touched[field] && clientErrors[field]) return clientErrors[field];
    if (!edited[field]) return state.fieldErrors?.[field];
    return undefined;
  }

  const touch = (field: SellField) =>
    setTouched((prev) => ({ ...prev, [field]: true }));
  function change(field: SellField, apply: () => void) {
    apply();
    setEdited((prev) => ({ ...prev, [field]: true }));
  }

  async function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (pending || uploading) return;
    if (Object.keys(clientErrors).length > 0) {
      setTouched({ title: true, price: true, stock: true, categoryId: true });
      return;
    }
    const formData = new FormData(e.currentTarget);
    const next = await run(() => saveListingAction(state, formData));
    setState(next);
    setEdited({});
    if (next.ok) toast.success("Đã lưu");
    else toast.error(next.error ?? next.message);
  }

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const input = e.target;
    const files = input.files;
    if (!files || files.length === 0) return;
    setUploading(true);
    setUploadError("");
    try {
      const newKeys = [...imageKeys];
      for (const file of Array.from(files)) {
        if (!file.type.startsWith("image/")) {
          throw new Error("Chỉ chấp nhận file hình ảnh.");
        }
        if (file.size > 10 * 1024 * 1024) {
          throw new Error("Kích thước file không vượt quá 10MB.");
        }
        const res = await getUploadUrlAction(file.type, file.name);
        if (!res.ok || !res.uploadUrl || !res.imageKey) {
          throw new Error(res.message || "Không xin được URL upload.");
        }
        const uploadRes = await fetch(res.uploadUrl, {
          method: "PUT",
          headers: { "Content-Type": file.type },
          body: file,
        });
        if (!uploadRes.ok) {
          throw new Error(`Upload thất bại: HTTP ${uploadRes.status}`);
        }
        newKeys.push(res.imageKey);
      }
      setImageKeys(newKeys);
    } catch (err: unknown) {
      setUploadError(
        err instanceof Error ? err.message : "Có lỗi xảy ra khi tải ảnh.",
      );
    } finally {
      setUploading(false);
      input.value = "";
    }
  }

  function patchVariant(key: string, patch: Partial<FormVariant>) {
    setRows((prev) =>
      prev.map((r) =>
        r.key === key ? { ...r, value: { ...r.value, ...patch } } : r,
      ),
    );
  }

  const serverBanner = !state.ok && (state.error ?? state.message);
  const sentVariants = variants.filter((v) => v.name.trim() !== "");

  // Created: a success Result replaces the form (no accidental re-submit).
  if (state.ok && !id) {
    return (
      <Result
        status="success"
        title="Đã đăng bán sản phẩm"
        subTitle={state.message}
        extra={
          <>
            <LinkButton href="/seller" variant="primary">
              Xem danh sách
            </LinkButton>
            {state.id && (
              <LinkButton href={`/listing/${state.id}`}>
                Xem sản phẩm
              </LinkButton>
            )}
          </>
        }
      />
    );
  }

  return (
    <form onSubmit={onSubmit} noValidate className="space-y-4">
      <input type="hidden" name="id" value={id} />
      <input type="hidden" name="imageKeys" value={JSON.stringify(imageKeys)} />
      <input
        type="hidden"
        name="variants"
        value={JSON.stringify(sentVariants)}
      />

      {serverBanner && <Alert type="error" description={serverBanner} />}
      {state.ok && id && <Alert type="success" description={state.message} />}

      <MagicListingAssist
        title={title}
        categoryId={categoryId}
        onApply={(v) => {
          if (v.title !== undefined)
            change("title", () => setTitle(v.title ?? ""));
          if (v.description !== undefined) setDescription(v.description);
          if (v.price !== undefined) {
            change("price", () => setPrice(String(v.price)));
          }
        }}
      />

      <fieldset
        disabled={pending}
        className="m-0 min-w-0 space-y-4 border-0 p-0"
      >
        <Section title="Thông tin cơ bản">
          <FormItem
            label="Tên sản phẩm"
            required
            status={errorOf("title") ? "error" : undefined}
            help={errorOf("title")}
          >
            <Input
              id="title"
              name="title"
              required
              value={title}
              onChange={(e) => change("title", () => setTitle(e.target.value))}
              onBlur={() => touch("title")}
              placeholder="VD: Điện thoại iPhone 15 Pro Max 256GB - Hàng Chính Hãng"
            />
          </FormItem>
          <FormItem
            label="Ngành hàng"
            required
            status={errorOf("categoryId") ? "error" : undefined}
            help={errorOf("categoryId")}
          >
            <Select
              id="categoryId"
              name="categoryId"
              value={categoryId}
              onChange={(e) =>
                change("categoryId", () => setCategoryId(e.target.value))
              }
              onBlur={() => touch("categoryId")}
            >
              <option value="">-- Chọn ngành hàng --</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.iconUrl ? `${c.iconUrl} ` : ""}
                  {c.name}
                </option>
              ))}
            </Select>
          </FormItem>
          <FormItem label="Mô tả chi tiết">
            <textarea
              id="description"
              name="description"
              rows={5}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              className={textareaClass}
              placeholder="Thông số kỹ thuật, chất liệu, kích thước, xuất xứ, chính sách bảo hành..."
            />
          </FormItem>
        </Section>

        <Section
          title="Hình ảnh"
          description="Ảnh vuông 1:1, tối thiểu 1 ảnh. Ảnh đầu tiên là ảnh bìa."
        >
          {imageKeys.length > 0 && (
            <ul className="grid grid-cols-2 gap-3 sm:grid-cols-4 md:grid-cols-6">
              {imageKeys.map((key, idx) => (
                <li key={key} className="relative">
                  <Image
                    src={getImageUrl(key)}
                    alt={`Ảnh sản phẩm ${idx + 1}`}
                    aspect="square"
                    loading={idx < EAGER_IMAGES ? "eager" : "lazy"}
                  />
                  {idx === 0 && (
                    <span className="absolute bottom-1 left-1">
                      <Tag color="primary">Ảnh bìa</Tag>
                    </span>
                  )}
                  <Button
                    size="xs"
                    variant="white"
                    className="absolute right-1 top-1"
                    aria-label={`Xoá ảnh ${idx + 1}`}
                    onClick={() =>
                      setImageKeys(imageKeys.filter((_, i) => i !== idx))
                    }
                  >
                    ✕
                  </Button>
                </li>
              ))}
            </ul>
          )}
          <label className="flex cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border-2 border-dashed border-border-strong p-6 text-center transition hover:border-action-primary focus-within:ring-2 focus-within:ring-focus-ring">
            <span className="text-sm font-medium text-text-primary">
              {uploading ? "Đang tải ảnh lên..." : "Chọn ảnh sản phẩm"}
            </span>
            <span className="text-xs text-text-secondary">
              Chọn nhiều ảnh cùng lúc, tối đa 10MB mỗi ảnh
            </span>
            <input
              type="file"
              accept="image/*"
              multiple
              disabled={uploading}
              onChange={handleFileChange}
              className="sr-only"
            />
          </label>
          {uploadError && <Alert type="error" description={uploadError} />}
        </Section>

        <Section title="Giá & tồn kho">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <FormItem
              label="Giá bán (₫)"
              required
              status={errorOf("price") ? "error" : undefined}
              help={errorOf("price")}
            >
              <Input
                id="price"
                name="price"
                type="number"
                min={0}
                value={price}
                onChange={(e) =>
                  change("price", () => setPrice(e.target.value))
                }
                onBlur={() => touch("price")}
                placeholder="VD: 29990000"
              />
            </FormItem>
            <FormItem
              label="Kho hàng"
              required
              status={errorOf("stock") ? "error" : undefined}
              help={errorOf("stock")}
            >
              <Input
                id="stock"
                name="stock"
                type="number"
                min={0}
                value={stock}
                onChange={(e) =>
                  change("stock", () => setStock(e.target.value))
                }
                onBlur={() => touch("stock")}
                placeholder="VD: 100"
              />
            </FormItem>
            <FormItem label="Đơn vị tiền tệ">
              <Select
                id="currency"
                name="currency"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
                options={[{ value: "VND", label: "VND (Việt Nam Đồng)" }]}
              />
            </FormItem>
          </div>
        </Section>

        <Section
          title="Phân loại"
          description="Tuỳ chọn: thêm màu, kích cỡ... với giá và tồn kho riêng."
        >
          {rows.map(({ key, value: v }) => (
            <div
              key={key}
              className="grid grid-cols-1 gap-3 rounded-xl border border-border-subtle p-3 sm:grid-cols-5"
            >
              <Input
                label="Tên phân loại"
                value={v.name}
                onChange={(e) => patchVariant(key, { name: e.target.value })}
              />
              <Input
                label="SKU"
                value={v.sku ?? ""}
                onChange={(e) => patchVariant(key, { sku: e.target.value })}
              />
              <Input
                label="Giá (₫)"
                type="number"
                min={0}
                value={v.price ?? ""}
                onChange={(e) =>
                  patchVariant(key, {
                    price:
                      e.target.value === ""
                        ? undefined
                        : Number(e.target.value),
                  })
                }
              />
              <Input
                label="Kho"
                type="number"
                min={0}
                value={v.stock ?? ""}
                onChange={(e) =>
                  patchVariant(key, {
                    stock:
                      e.target.value === ""
                        ? undefined
                        : Number(e.target.value),
                  })
                }
              />
              <div className="flex items-end">
                <Button
                  variant="ghost"
                  size="sm"
                  className="text-danger"
                  onClick={() => setRows(rows.filter((r) => r.key !== key))}
                >
                  Xoá phân loại
                </Button>
              </div>
            </div>
          ))}
          <Button
            variant="outline"
            size="sm"
            onClick={() =>
              setRows([...rows, { key: newRowKey(), value: { name: "" } }])
            }
          >
            Thêm phân loại
          </Button>
        </Section>

        <Section title="Trạng thái">
          <FormItem label="Hiển thị" group>
            <RadioGroup
              name="status"
              value={status}
              onChange={setStatus}
              options={STATUS_OPTIONS}
            />
          </FormItem>
        </Section>
      </fieldset>

      {/* Sticky on mobile so Save stays reachable; inline from md up. */}
      <div className="sticky bottom-0 z-20 -mx-4 flex items-center justify-end gap-3 border-t border-border-subtle bg-surface-card px-4 py-3 md:static md:mx-0 md:rounded-xl md:border">
        <Link
          href="/seller"
          className="rounded-lg border border-border-strong bg-surface-card px-4 py-2 text-sm font-medium text-text-primary shadow-sm transition hover:bg-surface-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring"
        >
          Huỷ
        </Link>
        <Button type="submit" isLoading={pending} disabled={uploading}>
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}
