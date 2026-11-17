"use client";

import { useState } from "react";

import { getUploadUrlAction } from "@/features/listing/actions";

const MAX_BYTES = 10 * 1024 * 1024;
const TYPES = ["image/jpeg", "image/png", "image/webp"];

export function ShopBannerField({
  value,
  onChange,
}: {
  value: string;
  onChange: (url: string) => void;
}) {
  const [error, setError] = useState("");
  const [uploading, setUploading] = useState(false);

  async function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const input = e.target;
    const file = input.files?.[0];
    if (!file) return;
    setError("");
    if (!TYPES.includes(file.type)) {
      setError("Chỉ chấp nhận ảnh JPEG, PNG hoặc WebP.");
      return;
    }
    if (file.size > MAX_BYTES) {
      setError("Kích thước file không vượt quá 10MB.");
      return;
    }
    setUploading(true);
    try {
      const res = await getUploadUrlAction(file.type, file.name);
      if (!res.ok || !res.uploadUrl || !res.publicUrl) {
        throw new Error(res.message || "Không xin được URL upload.");
      }
      const up = await fetch(res.uploadUrl, {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      });
      if (!up.ok) throw new Error(`Upload thất bại: HTTP ${up.status}`);
      onChange(res.publicUrl);
    } catch (err: unknown) {
      setError(
        err instanceof Error ? err.message : "Upload thất bại. Thử lại sau.",
      );
    } finally {
      setUploading(false);
      input.value = "";
    }
  }

  return (
    <div className="space-y-2">
      {value && (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={value}
          alt="Banner gian hàng"
          className="max-h-40 w-full rounded-lg object-cover"
        />
      )}
      <label className="block text-sm font-medium text-text-primary">
        Banner gian hàng
        <input
          type="file"
          accept="image/jpeg,image/png,image/webp"
          disabled={uploading}
          onChange={handleChange}
          className="mt-1 block text-sm"
        />
      </label>
      {uploading && <p className="text-xs">Đang tải ảnh lên...</p>}
      {value && (
        <button
          type="button"
          onClick={() => onChange("")}
          className="text-sm text-action-danger underline"
        >
          Xóa banner
        </button>
      )}
      {error && (
        <p role="alert" className="text-sm text-action-danger">
          {error}
        </p>
      )}
    </div>
  );
}
