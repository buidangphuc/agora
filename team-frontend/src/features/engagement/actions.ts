"use server";

import { revalidatePath } from "next/cache";

import {
  type CheckInResult,
  type ViewCollection,
  addFavorite,
  addToCollection,
  checkIn,
  createCollection,
  followSeller,
  listCollections,
  removeFavorite,
  removeFromCollection,
  unfollowSeller,
} from "@/lib/gateway/engagement";

export async function addFavoriteAction(id: string): Promise<void> {
  await addFavorite(id);
  revalidatePath(`/listing/${id}`);
  revalidatePath("/favorites");
}

export async function removeFavoriteAction(id: string): Promise<void> {
  await removeFavorite(id);
  revalidatePath(`/listing/${id}`);
  revalidatePath("/favorites");
}

// ── Wishlist collections ────────────────────────────────────────────────────

export async function createCollectionAction(
  name: string,
): Promise<{ ok: boolean; collection?: ViewCollection; message?: string }> {
  if (!name.trim()) {
    return { ok: false, message: "Tên bộ sưu tập không được để trống." };
  }
  try {
    const collection = await createCollection(name);
    revalidatePath("/favorites");
    return { ok: true, collection };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Tạo bộ sưu tập thất bại.",
    };
  }
}

export async function listCollectionsAction(): Promise<ViewCollection[]> {
  return listCollections();
}

export async function addToCollectionAction(
  collectionId: string,
  listingId: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await addToCollection(collectionId, listingId);
    revalidatePath("/favorites");
    revalidatePath(`/listing/${listingId}`);
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Thêm vào bộ sưu tập thất bại.",
    };
  }
}

export async function removeFromCollectionAction(
  collectionId: string,
  listingId: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await removeFromCollection(collectionId, listingId);
    revalidatePath("/favorites");
    revalidatePath(`/listing/${listingId}`);
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message:
        err instanceof Error ? err.message : "Xóa khỏi bộ sưu tập thất bại.",
    };
  }
}

// ── Loyalty / daily check-in ────────────────────────────────────────────────

export async function checkInAction(): Promise<{
  ok: boolean;
  result?: CheckInResult;
  message?: string;
}> {
  try {
    const result = await checkIn();
    revalidatePath("/");
    return { ok: true, result };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Điểm danh thất bại.",
    };
  }
}

// ── Follow sellers ──────────────────────────────────────────────────────────

export async function followSellerAction(
  sellerId: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await followSeller(sellerId);
    revalidatePath(`/shop/${sellerId}`);
    revalidatePath("/account/following");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Theo dõi shop thất bại.",
    };
  }
}

export async function unfollowSellerAction(
  sellerId: string,
): Promise<{ ok: boolean; message?: string }> {
  try {
    await unfollowSeller(sellerId);
    revalidatePath(`/shop/${sellerId}`);
    revalidatePath("/account/following");
    return { ok: true };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Bỏ theo dõi thất bại.",
    };
  }
}
