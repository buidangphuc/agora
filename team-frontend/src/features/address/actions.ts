"use server";

import { revalidatePath } from "next/cache";

import { errorMessage } from "@/features/account/action-error";
import { type ActionResult, fail, ok } from "@/lib/action-result";
import {
  type AddressInput,
  type ViewAddress,
  createAddress,
  deleteAddress,
  setDefaultAddress,
  updateAddress,
} from "@/lib/gateway/addresses";

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

function isComplete(input: AddressInput): boolean {
  return Boolean(
    input.recipientName && input.phone && input.street && input.city,
  );
}

function revalidateAddresses() {
  revalidatePath("/account/addresses");
  revalidatePath("/checkout");
}

export async function createAddressAction(
  formData: FormData,
): Promise<ActionResult<ViewAddress>> {
  const input = readAddressInput(formData);
  if (!isComplete(input)) {
    return fail("Vui lòng điền đầy đủ các trường bắt buộc.");
  }
  try {
    const address = await createAddress(input);
    revalidateAddresses();
    return ok(address);
  } catch (err: unknown) {
    return fail(errorMessage(err, "Thêm địa chỉ thất bại."));
  }
}

export async function updateAddressAction(
  id: string,
  formData: FormData,
): Promise<ActionResult<ViewAddress>> {
  const input = readAddressInput(formData);
  if (!isComplete(input)) {
    return fail("Vui lòng điền đầy đủ các trường bắt buộc.");
  }
  try {
    const address = await updateAddress(id, input);
    revalidateAddresses();
    return ok(address);
  } catch (err: unknown) {
    return fail(errorMessage(err, "Cập nhật địa chỉ thất bại."));
  }
}

export async function deleteAddressAction(id: string): Promise<ActionResult> {
  try {
    await deleteAddress(id);
    revalidateAddresses();
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Xóa địa chỉ thất bại."));
  }
}

export async function setDefaultAddressAction(
  id: string,
): Promise<ActionResult> {
  try {
    await setDefaultAddress(id);
    revalidateAddresses();
    return ok();
  } catch (err: unknown) {
    return fail(errorMessage(err, "Đặt địa chỉ mặc định thất bại."));
  }
}
