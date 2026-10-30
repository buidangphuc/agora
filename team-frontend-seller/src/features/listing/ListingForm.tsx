"use client";

import { useEffect, useRef, useState } from "react";

import { Alert } from "@/components/ui/Alert";
import { Button } from "@/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/Card";
import { FormItem } from "@/components/ui/FormItem";
import { Image } from "@/components/ui/Image";
import { Input } from "@/components/ui/Input";
import { Link } from "@/components/ui/Link";
import { RadioGroup } from "@/components/ui/Radio";
import { Result } from "@/components/ui/Result";
import { Select } from "@/components/ui/Select";
import { Tag } from "@/components/ui/Tag";
import { useToast } from "@/components/ui/ToastProvider";
import { LinkButton } from "@/features/seller/LinkButton";
import { usePending } from "@/features/seller/usePending";
import { trackAiInteraction } from "@/lib/analytics/ai";
import { buyerAppUrl } from "@/lib/apps";
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
  "block min-h-touch w-full rounded-xl border border-border-strong bg-surface-card px-4 py-3 text-base text-text-primary transition duration-150 placeholder:text-text-disabled focus-visible:border-action-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus-ring aria-invalid:border-danger";

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
      <CardHeader>
        <div className="min-w-0">
          <CardTitle>{title}</CardTitle>
          {description && <CardDescription>{description}</CardDescription>}
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-5">{children}</div>
      </CardContent>
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
  readOnly = false,
}: {
  listingId?: string;
  defaults?: ListingDefaults;
  categories?: CategoryOption[];
  submitLabel?: string;
  /** Banned or held listings: fields disabled, no Save. */
  readOnly?: boolean;
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

  // Magic Listing: fields the seller applied from the current draft, and
  // whether the one "edited" event of this draft was already reported.
  const magicApplied = useRef<{ fields: Set<string>; reported: boolean }>({
    fields: new Set(),
    reported: false,
  });
  function noteEdit(field: "title" | "description" | "price") {
    const m = magicApplied.current;
    if (m.reported || !m.fields.has(field)) return;
    m.reported = true;
    trackAiInteraction("magic_listing", "edited", listingId);
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
              <LinkButton href={buyerAppUrl(`/listing/${state.id}`)}>
                Xem sản phẩm
              </LinkButton>
            )}
          </>
        }
      />
    );
  }

  return (
    <form onSubmit={onSubmit} noValidate className="space-y-6">
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
        listingId={listingId}
        onDraft={() => {
          magicApplied.current = { fields: new Set(), reported: false };
        }}
        onApply={(v) => {
          for (const f of ["title", "description", "price"] as const) {
            if (v[f] !== undefined) magicApplied.current.fields.add(f);
          }
          if (v.title !== undefined)
            change("title", () => setTitle(v.title ?? ""));
          if (v.description !== undefined) setDescription(v.description);
          if (v.price !== undefined) {
            change("price", () => setPrice(String(v.price)));
          }
        }}
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3 lg:items-start">
        <fieldset
          disabled={pending || readOnly}
          className="m-0 min-w-0 space-y-6 border-0 p-0 lg:col-span-2"
        >
          <Section title="Thông tin cơ bản">
            <Input
              label="Tên sản phẩm"
              error={errorOf("title")}
              id="title"
              name="title"
              required
              value={title}
              onChange={(e) => {
                noteEdit("title");
                change("title", () => setTitle(e.target.value));
              }}
              onBlur={() => touch("title")}
              placeholder="VD: Điện thoại iPhone 15 Pro Max 256GB - Hàng Chính Hãng"
            />
            <Select
              label="Ngành hàng"
              required
              error={errorOf("categoryId")}
              id="categoryId"
              name="categoryId"
              value={categoryId}
              onChange={(e) =>
                change("categoryId", () => setCategoryId(e.target.value))
              }
              onBlur={() => touch("categoryId")}
              placeholder="-- Chọn ngành hàng --"
              options={categories.map((c) => ({
                value: c.id,
                label: `${c.iconUrl ? `${c.iconUrl} ` : ""}${c.name}`,
              }))}
            />
            <FormItem label="Mô tả chi tiết">
              <textarea
                id="description"
                name="description"
                rows={5}
                value={description}
                onChange={(e) => {
                  noteEdit("description");
                  setDescription(e.target.value);
                }}
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
              <ul className="grid grid-cols-2 gap-4 sm:grid-cols-4 xl:grid-cols-5">
                {imageKeys.map((key, idx) => (
                  <li key={key} className="relative">
                    <Image
                      src={getImageUrl(key)}
                      alt={`Ảnh sản phẩm ${idx + 1}`}
                      aspect="square"
                      loading={idx < EAGER_IMAGES ? "eager" : "lazy"}
                    />
                    {idx === 0 && (
                      <span className="absolute bottom-2 left-2">
                        <Tag color="primary">Ảnh bìa</Tag>
                      </span>
                    )}
                    <span className="absolute right-2 top-2">
                      <Button
                        size="xs"
                        variant="white"
                        aria-label={`Xoá ảnh ${idx + 1}`}
                        onClick={() =>
                          setImageKeys(imageKeys.filter((_, i) => i !== idx))
                        }
                      >
                        ✕
                      </Button>
                    </span>
                  </li>
                ))}
              </ul>
            )}
            <label className="flex min-h-touch cursor-pointer flex-col items-center justify-center gap-2 rounded-2xl border-2 border-dashed border-border-strong bg-surface-muted px-6 py-10 text-center transition hover:border-action-primary focus-within:ring-2 focus-within:ring-focus-ring">
              <span className="text-base font-semibold text-text-primary">
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
            <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 xl:grid-cols-3">
              <Input
                required
                label="Giá bán (₫)"
                error={errorOf("price")}
                id="price"
                name="price"
                type="number"
                min={0}
                value={price}
                onChange={(e) => {
                  noteEdit("price");
                  change("price", () => setPrice(e.target.value));
                }}
                onBlur={() => touch("price")}
                placeholder="VD: 29990000"
              />
              <Input
                required
                label="Kho hàng"
                error={errorOf("stock")}
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
              <Select
                label="Đơn vị tiền tệ"
                id="currency"
                name="currency"
                value={currency}
                onChange={(e) => setCurrency(e.target.value)}
                options={[{ value: "VND", label: "VND (Việt Nam Đồng)" }]}
              />
            </div>
          </Section>

          <Section
            title="Phân loại"
            description="Tuỳ chọn: thêm màu, kích cỡ... với giá và tồn kho riêng."
          >
            {rows.map(({ key, value: v }) => (
              <div
                key={key}
                className="grid grid-cols-1 gap-4 rounded-2xl border border-border-subtle bg-surface-muted p-4 sm:grid-cols-2 xl:grid-cols-4"
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
                <div className="flex items-end sm:col-span-2 xl:col-span-4 xl:justify-end">
                  <Button
                    variant="danger"
                    size="sm"
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
              fullWidth="mobile"
              onClick={() =>
                setRows([...rows, { key: newRowKey(), value: { name: "" } }])
              }
            >
              Thêm phân loại
            </Button>
          </Section>
        </fieldset>
        <fieldset
          disabled={pending || readOnly}
          className="m-0 min-w-0 space-y-6 border-0 p-0 lg:sticky lg:top-6"
        >
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
      </div>

      {/* Sticky on mobile so Save stays reachable; inline from md up. */}
      <div className="sticky bottom-0 z-20 -mx-4 flex items-center justify-end gap-3 border-t border-border-subtle bg-surface-card px-4 py-3 shadow-card md:static md:mx-0 md:rounded-2xl md:border md:px-6 md:py-4">
        <Link href="/seller" variant="outline">
          Huỷ
        </Link>
        {!readOnly && (
          <Button type="submit" isLoading={pending} disabled={uploading}>
            {submitLabel}
          </Button>
        )}
      </div>
    </form>
  );
}
