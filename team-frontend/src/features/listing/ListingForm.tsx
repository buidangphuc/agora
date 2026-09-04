"use client";

import { useState } from "react";
import { useFormState, useFormStatus } from "react-dom";

import { getImageUrl } from "@/lib/media";
import {
  type SellState,
  getUploadUrlAction,
  magicListingAction,
  saveListingAction,
} from "./actions";

const initialSellState: SellState = { ok: false, message: "" };

const label =
  "block text-xs font-semibold text-gray-700 uppercase tracking-wider";
const field =
  "mt-1.5 w-full rounded-xs border border-gray-300 px-3.5 py-2 text-xs text-gray-900 outline-none focus:border-brand focus:ring-1 focus:ring-brand";

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

function SubmitButton({
  text,
  disabled,
}: { text: string; disabled?: boolean }) {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      disabled={pending || disabled}
      className="rounded-xs bg-brand px-8 py-2.5 text-xs font-bold uppercase tracking-wider text-white shadow-xs hover:bg-brand-dark transition disabled:opacity-60"
    >
      {pending ? "Đang lưu sản phẩm..." : text}
    </button>
  );
}

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
  const [state, formAction] = useFormState(saveListingAction, initialSellState);
  const [imageKeys, setImageKeys] = useState<string[]>(
    Array.isArray(defaults.imageKeys) ? defaults.imageKeys : [],
  );
  const [variants, setVariants] = useState<FormVariant[]>(
    Array.isArray(defaults.variants) ? defaults.variants : [],
  );
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const [aiLoading, setAiLoading] = useState(false);
  const [aiError, setAiError] = useState("");

  // Fill the form from team-ai MagicListing (through the gateway).
  const handleMagicFill = async () => {
    setAiError("");
    const titleInput = document.getElementById(
      "title",
    ) as HTMLInputElement | null;
    const descInput = document.getElementById(
      "description",
    ) as HTMLTextAreaElement | null;
    const priceInput = document.getElementById(
      "price",
    ) as HTMLInputElement | null;
    const categoryInput = document.getElementById(
      "categoryId",
    ) as HTMLInputElement | null;

    const titleHint =
      titleInput?.value?.trim() ||
      "Điện thoại iPhone 15 Pro Max 256GB - Hàng Chính Hãng";
    if (titleInput && !titleInput.value) {
      titleInput.value = titleHint;
    }

    setAiLoading(true);
    const res = await magicListingAction(titleHint, categoryInput?.value ?? "");
    setAiLoading(false);

    if (!res.ok || !res.result) {
      setAiError(res.message || "AI tạm thời không phản hồi.");
      return;
    }

    const r = res.result;
    if (titleInput && r.generatedTitle) {
      titleInput.value = r.generatedTitle;
      titleInput.dispatchEvent(new Event("input", { bubbles: true }));
    }
    if (descInput && r.generatedDescription) {
      descInput.value = r.generatedDescription;
      descInput.dispatchEvent(new Event("input", { bubbles: true }));
    }
    if (
      priceInput &&
      r.suggestedPriceMin > 0 &&
      (!priceInput.value || Number(priceInput.value) <= 100000)
    ) {
      priceInput.value = String(r.suggestedPriceMin);
      priceInput.dispatchEvent(new Event("input", { bubbles: true }));
    }
  };

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
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
      e.target.value = "";
    }
  }

  function handleRemoveImage(idxToRemove: number) {
    setImageKeys(imageKeys.filter((_, idx) => idx !== idxToRemove));
  }

  return (
    <form
      action={formAction}
      className="space-y-6 rounded-xs border border-gray-200 bg-white p-6 md:p-8 shadow-2xs"
    >
      {/* Hidden inputs */}
      <input type="hidden" name="id" value={listingId ?? defaults.id ?? ""} />
      <input type="hidden" name="imageKeys" value={JSON.stringify(imageKeys)} />
      <input type="hidden" name="variants" value={JSON.stringify(variants)} />

      {/* ── Image Upload Area ── */}
      <div>
        <span className={label}>
          📸 Hình ảnh sản phẩm (Tỉ lệ 1:1, Tối thiểu 1 ảnh)
        </span>
        <div className="mt-2">
          {imageKeys.length > 0 && (
            <div className="mb-3 grid grid-cols-2 gap-3 sm:grid-cols-4 md:grid-cols-6">
              {imageKeys.map((key, idx) => (
                <div
                  key={key}
                  className="group relative aspect-square overflow-hidden rounded-xs border border-gray-200 bg-gray-50 shadow-2xs"
                >
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={getImageUrl(key)}
                    alt={`Preview ${idx + 1}`}
                    className="h-full w-full object-cover"
                  />
                  <button
                    type="button"
                    onClick={() => handleRemoveImage(idx)}
                    className="absolute right-1 top-1 rounded-full bg-red-600 p-1 text-[10px] text-white opacity-90 shadow-sm transition hover:bg-red-700"
                    title="Xóa ảnh"
                  >
                    ✕
                  </button>
                  {idx === 0 && (
                    <span className="absolute bottom-1 left-1 rounded bg-brand px-1.5 py-0.2 text-[9px] font-bold text-white uppercase">
                      Ảnh bìa
                    </span>
                  )}
                </div>
              ))}
            </div>
          )}

          <label className="flex cursor-pointer flex-col items-center justify-center rounded-xs border-2 border-dashed border-gray-300 p-6 text-center transition hover:border-brand hover:bg-orange-50/20">
            <span className="text-3xl">🖼️</span>
            <span className="mt-2 text-xs font-bold text-gray-800">
              {uploading
                ? "Đang tải ảnh lên máy chủ..."
                : "+ Nhấn để chọn hoặc kéo thả nhiều ảnh sản phẩm vào đây"}
            </span>
            <input
              type="file"
              accept="image/*"
              multiple
              disabled={uploading}
              onChange={handleFileChange}
              className="hidden"
            />
          </label>
          {uploadError && (
            <p className="mt-2 text-xs text-red-600">{uploadError}</p>
          )}
        </div>
      </div>

      {/* ── Title & AI Magic Fill ── */}
      <div>
        <div className="flex items-center justify-between">
          <label className={label} htmlFor="title">
            Tên sản phẩm <span className="text-red-500">*</span>
          </label>
          <button
            type="button"
            onClick={handleMagicFill}
            disabled={aiLoading}
            className="px-3 py-1 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-700 hover:to-indigo-700 disabled:opacity-50 text-white rounded-lg text-xs font-bold transition shadow-xs flex items-center gap-1.5"
          >
            <span>✨</span>
            <span>
              {aiLoading ? "Đang tạo..." : "AI Tạo Mô Tả & Gợi Ý Giá"}
            </span>
          </button>
        </div>
        {aiError && <p className="mt-1 text-xs text-red-600">{aiError}</p>}
        <input
          id="title"
          name="title"
          required
          defaultValue={defaults.title}
          className={field}
          placeholder="VD: Điện thoại iPhone 15 Pro Max 256GB - Hàng Chính Hãng VN/A"
        />
      </div>

      {/* ── Category & Status ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label className={label} htmlFor="categoryId">
            Ngành hàng danh mục <span className="text-red-500">*</span>
          </label>
          <select
            id="categoryId"
            name="categoryId"
            defaultValue={defaults.categoryId ?? ""}
            className={field}
          >
            <option value="">-- Chọn ngành hàng --</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.iconUrl ? `${c.iconUrl} ` : ""}
                {c.name}
              </option>
            ))}
          </select>
        </div>

        <div>
          <label className={label} htmlFor="status">
            Trạng thái hiển thị
          </label>
          <select
            id="status"
            name="status"
            defaultValue={defaults.status ?? "published"}
            className={field}
          >
            <option value="published">Đang bán (Hiển thị công khai)</option>
            <option value="draft">Lưu bản nháp</option>
          </select>
        </div>
      </div>

      {/* ── Price & Stock ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div>
          <label className={label} htmlFor="price">
            Giá bán (₫) <span className="text-red-500">*</span>
          </label>
          <input
            id="price"
            name="price"
            type="number"
            min={0}
            defaultValue={defaults.price ?? 100000}
            className={field}
            placeholder="VD: 29990000"
          />
        </div>

        <div>
          <label className={label} htmlFor="stock">
            Kho hàng (Số lượng có sẵn) <span className="text-red-500">*</span>
          </label>
          <input
            id="stock"
            name="stock"
            type="number"
            min={0}
            defaultValue={defaults.stock ?? 100}
            className={field}
            placeholder="VD: 100"
          />
        </div>

        <div>
          <label className={label} htmlFor="currency">
            Đơn vị tiền tệ
          </label>
          <select
            id="currency"
            name="currency"
            defaultValue={defaults.currency ?? "VND"}
            className={field}
          >
            <option value="VND">VND (Việt Nam Đồng)</option>
          </select>
        </div>
      </div>

      {/* ── Description ── */}
      <div>
        <label className={label} htmlFor="description">
          Mô tả chi tiết sản phẩm
        </label>
        <textarea
          id="description"
          name="description"
          rows={5}
          defaultValue={defaults.description}
          className={field}
          placeholder="Mô tả thông số kỹ thuật, chất liệu, kích thước, nguồn gốc xuất xứ, chính sách bảo hành chính hãng..."
        />
      </div>

      {/* ── Submit CTA ── */}
      <div className="flex items-center gap-4 pt-4 border-t">
        <SubmitButton text={submitLabel} disabled={uploading} />
        {state.message && (
          <p
            className={`text-xs font-semibold ${
              state.ok ? "text-emerald-600" : "text-red-600"
            }`}
          >
            {state.message}
          </p>
        )}
      </div>
    </form>
  );
}
