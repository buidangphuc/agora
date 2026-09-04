"use server";

import { revalidatePath } from "next/cache";

import {
  type AddressInput,
  createAddress,
  deleteAddress,
  setDefaultAddress,
  updateAddress,
} from "@/lib/gateway/addresses";

export interface AddressState {
  ok: boolean;
  message: string;
}

function readAddressInput(formData: FormData): AddressInput {
  return {
    recipientName: String(formData.get("recipientName") ?? "").trim(),
    phone: String(formData.get("phone") ?? "").trim(),
    street: String(formData.get("street") ?? "").trim(),
    ward: String(formData.get("ward") ?? "").trim(),
    district: String(formData.get("district") ?? "").trim(),
    city: String(formData.get("city") ?? "").trim(),
    isDefault:
      formData.get("isDefault") === "on" ||
      formData.get("isDefault") === "true",
  };
}

export async function createAddressAction(
  _prev: AddressState,
  formData: FormData,
): Promise<AddressState> {
  const input = readAddressInput(formData);
  if (!input.recipientName || !input.phone || !input.street || !input.city) {
    return { ok: false, message: "Vui lòng điền đầy đủ các trường bắt buộc." };
  }
  try {
    await createAddress(input);
    revalidatePath("/account/addresses");
    revalidatePath("/checkout");
    return { ok: true, message: "Đã thêm địa chỉ thành công." };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Thêm địa chỉ thất bại.",
    };
  }
}

export async function updateAddressAction(
  id: string,
  _prev: AddressState,
  formData: FormData,
): Promise<AddressState> {
  const input = readAddressInput(formData);
  if (!input.recipientName || !input.phone || !input.street || !input.city) {
    return { ok: false, message: "Vui lòng điền đầy đủ các trường bắt buộc." };
  }
  try {
    await updateAddress(id, input);
    revalidatePath("/account/addresses");
    revalidatePath("/checkout");
    return { ok: true, message: "Đã cập nhật địa chỉ thành công." };
  } catch (err: unknown) {
    return {
      ok: false,
      message: err instanceof Error ? err.message : "Cập nhật thất bại.",
    };
  }
}

export async function deleteAddressAction(id: string): Promise<void> {
  await deleteAddress(id);
  revalidatePath("/account/addresses");
  revalidatePath("/checkout");
}

export async function setDefaultAddressAction(id: string): Promise<void> {
  await setDefaultAddress(id);
  revalidatePath("/account/addresses");
  revalidatePath("/checkout");
}
