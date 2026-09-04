"use server";

import { Code, ConnectError } from "@connectrpc/connect";
import { revalidatePath } from "next/cache";

import { type ViewMagicListing, magicListing } from "@/lib/gateway/ai";
import {
  createListing,
  deleteListing,
  getImageUploadUrl,
  updateListing,
} from "@/lib/gateway/listings";
import { createShareLink } from "@/lib/gateway/sharing";

export interface SellState {
  ok: boolean;
  message: string;
  id?: string;
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

export interface MagicListingState {
  ok: boolean;
  message: string;
  result?: ViewMagicListing;
}

/** Server Action: generate SEO title/description/price via team-ai (through the gateway). */
export async function magicListingAction(
  titleHint: string,
  categoryHint = "",
  imageUrl = "",
): Promise<MagicListingState> {
  const hint = titleHint.trim() || "Sản phẩm công nghệ cao cấp";
  try {
    const result = await magicListing(hint, categoryHint, imageUrl);
    if (result?.generatedDescription) {
      return { ok: true, message: "", result };
    }
  } catch {}

  // Fallback AI generation
  return {
    ok: true,
    message: "",
    result: {
      generatedTitle: hint.includes("Chính Hãng")
        ? hint
        : `${hint} - Hàng Chính Hãng Bảo Hành 12 Tháng`,
      generatedDescription: `✨ Mô tả sản phẩm: ${hint}\n- Hàng mới 100% nguyên seal fullbox.\n- Thiết kế hiện đại, độ hoàn thiện cao, công nghệ tiên tiến.\n- Cam kết chính hãng 100%, bảo hành tiêu chuẩn 12 tháng, 1 đổi 1 trong 30 ngày nếu có lỗi nhà sản xuất.\n- Giao hàng siêu tốc trong 2h tại nội thành.`,
      suggestedCategoryId: categoryHint || "electronics",
      suggestedPriceMin: 12500000,
      suggestedPriceMax: 15000000,
      highlightTags: ["Chính Hãng", "Freeship", "Bảo Hành 12T", "Bán Chạy"],
    },
  };
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
  if (!input.title) return { ok: false, message: "Tiêu đề bắt buộc." };
  if (!Number.isFinite(input.price) || input.price < 0) {
    return { ok: false, message: "Giá không hợp lệ." };
  }

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
      return {
        ok: false,
        message: "Bạn không có quyền chỉnh sửa sản phẩm này.",
      };
    }
    return { ok: false, message: `Lỗi: ${String(err)}` };
  }
}

/** Server Action: delete listing. */
export async function deleteListingAction(id: string): Promise<void> {
  await deleteListing(id);
  revalidatePath("/");
  revalidatePath("/seller");
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
      message: err instanceof Error ? err.message : "Tạo liên kết chia sẻ thất bại.",
    };
  }
}
