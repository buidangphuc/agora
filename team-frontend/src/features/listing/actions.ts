"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { type ActionResult, fail, ok } from "@/lib/action-result";
import { type ViewMagicListing, magicListing } from "@/lib/gateway/ai";
import {
  createListing,
  deleteListing,
  getImageUploadUrl,
  updateListing,
} from "@/lib/gateway/listings";
import { createShareLink } from "@/lib/gateway/sharing";

export type SellField = "title" | "price" | "stock" | "categoryId";

export interface SellState {
  ok: boolean;
  /** Success confirmation, or the failure text (mirrored in `error`). */
  message: string;
  /** Failure text; the field name `error` of the shared mutation contract. */
  error?: string;
  /** Server-side validation errors keyed by the form field they belong to. */
  fieldErrors?: Partial<Record<SellField, string>>;
  id?: string;
}

function failState(
  message: string,
  fieldErrors?: SellState["fieldErrors"],
): SellState {
  return { ok: false, message, error: message, fieldErrors };
}

function readInput(formData: FormData) {
  const imageKeysRaw = formData.get("imageKeys");
  let imageKeys: string[] = [];
  if (typeof imageKeysRaw === "string" && imageKeysRaw.trim() !== "") {
    try {
      imageKeys = JSON.parse(imageKeysRaw);
    } catch {
      imageKeys = imageKeysRaw
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
    }
  }
  const variantsRaw = formData.get("variants");
  let variants = [];
  if (typeof variantsRaw === "string" && variantsRaw.trim() !== "") {
    try {
      variants = JSON.parse(variantsRaw);
    } catch {}
  }
  return {
    title: String(formData.get("title") ?? "").trim(),
    description: String(formData.get("description") ?? "").trim(),
    price: Number(formData.get("price") ?? 0),
    currency: String(formData.get("currency") ?? "VND"),
    status: String(formData.get("status") ?? "published"),
    imageKeys,
    categoryId: String(formData.get("categoryId") ?? "").trim(),
    stock: Number(formData.get("stock") ?? 0),
    variants,
  };
}

/**
 * Server Action: generate SEO title/description/price via team-ai (through the
 * gateway). There is no local fallback: when the AI call fails the seller gets
 * an error and keeps editing by hand (no invented suggestions).
 */
export async function magicListingAction(
  titleHint: string,
  categoryHint = "",
  imageUrl = "",
): Promise<ActionResult<ViewMagicListing>> {
  const hint = titleHint.trim();
  if (!hint) return fail("Nhập tên sản phẩm trước khi dùng gợi ý AI.");
  try {
    const result = await magicListing(hint, categoryHint, imageUrl);
    if (result?.generatedDescription || result?.generatedTitle) {
      return ok(result);
    }
    return fail("AI chưa có gợi ý cho sản phẩm này.");
  } catch {
    return fail("AI tạm thời không phản hồi. Vui lòng thử lại.");
  }
}

/** Server Action: get a presigned S3 PUT URL for uploading an image. */
export async function getUploadUrlAction(
  contentType: string,
  filename?: string,
) {
  try {
    const res = await getImageUploadUrl(contentType, filename);
    return { ok: true, message: "", ...res };
  } catch (err) {
    return {
      ok: false,
      message: String(err),
      uploadUrl: "",
      imageKey: "",
      publicUrl: "",
    };
  }
}

/** Unified Server Action: create or update listing. */
export async function saveListingAction(
  _prev: SellState,
  formData: FormData,
): Promise<SellState> {
  const id = String(formData.get("id") ?? "").trim();
  const input = readInput(formData);

  const fieldErrors: NonNullable<SellState["fieldErrors"]> = {};
  if (!input.title) fieldErrors.title = "Tiêu đề bắt buộc.";
  if (!Number.isFinite(input.price) || input.price <= 0) {
    fieldErrors.price = "Giá không hợp lệ.";
  }
  if (!Number.isFinite(input.stock) || input.stock < 0) {
    fieldErrors.stock = "Tồn kho không hợp lệ.";
  }
  if (!input.categoryId) fieldErrors.categoryId = "Chọn ngành hàng.";
  const firstError = Object.values(fieldErrors)[0];
  if (firstError) return failState(firstError, fieldErrors);

  try {
    if (id) {
      const listing = await updateListing(id, input);
      revalidatePath("/");
      revalidatePath("/seller");
      revalidatePath(`/listing/${id}`);
      return {
        ok: true,
        message: `✓ Đã cập nhật thành công "${listing.title}"!`,
        id: listing.id,
      };
    }

    const listing = await createListing(input);
    revalidatePath("/");
    revalidatePath("/seller");
    return {
      ok: true,
      message: `✓ Đã đăng bán thành công "${listing.title}"!`,
      id: listing.id,
    };
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.PermissionDenied) {
      return failState("Bạn không có quyền chỉnh sửa sản phẩm này.");
    }
    return failState(`Lỗi: ${String(err)}`);
  }
}

/** Server Action: delete listing. */
export async function deleteListingAction(id: string): Promise<ActionResult> {
  try {
    await deleteListing(id);
  } catch (err) {
    return fail(
      err instanceof Error && err.message
        ? err.message
        : "Xoá sản phẩm thất bại.",
    );
  }
  revalidatePath("/");
  revalidatePath("/seller");
  return ok();
}

/**
 * Server Action: mint a short share link for a target (e.g. a listing). Wired to
 * team-sharing SharingService through the gateway. Returns the short code.
 */
export async function createShareLinkAction(
  targetType: string,
  targetId: string,
): Promise<{ ok: boolean; shortCode?: string; message?: string }> {
  try {
    const shortCode = await createShareLink(targetType, targetId);
    return { ok: true, shortCode };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Tạo liên kết chia sẻ thất bại.",
    };
  }
}
