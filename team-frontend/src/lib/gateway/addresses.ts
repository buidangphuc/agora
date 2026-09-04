import "server-only";

import type { Address } from "@/generated/platform/identity/v1/identity_pb.js";
import { makeClients } from "./client.js";
import { getToken } from "./session.js";

function gateway() {
  return makeClients(getToken());
}

export interface ViewAddress {
  id: string;
  userId: string;
  recipientName: string;
  phone: string;
  street: string;
  ward: string;
  district: string;
  city: string;
  isDefault: boolean;
}

export interface AddressInput {
  recipientName: string;
  phone: string;
  street: string;
  ward?: string;
  district?: string;
  city: string;
  isDefault?: boolean;
}

function mapAddress(a: Address): ViewAddress {
  return {
    id: a.id,
    userId: a.userId,
    recipientName: a.recipientName,
    phone: a.phone,
    street: a.street,
    ward: a.ward,
    district: a.district,
    city: a.city,
    isDefault: a.isDefault,
  };
}

export async function listAddresses(): Promise<ViewAddress[]> {
  try {
    const res = await gateway().address.listAddresses({});
    return res.addresses.map(mapAddress);
  } catch {
    return [];
  }
}

export async function createAddress(input: AddressInput): Promise<ViewAddress> {
  const res = await gateway().address.createAddress({
    recipientName: input.recipientName,
    phone: input.phone,
    street: input.street,
    ward: input.ward ?? "",
    district: input.district ?? "",
    city: input.city,
    isDefault: input.isDefault ?? false,
  });
  if (!res.address) throw new Error("create address failed");
  return mapAddress(res.address);
}

export async function updateAddress(
  id: string,
  input: AddressInput,
): Promise<ViewAddress> {
  const res = await gateway().address.updateAddress({
    id,
    recipientName: input.recipientName,
    phone: input.phone,
    street: input.street,
    ward: input.ward ?? "",
    district: input.district ?? "",
    city: input.city,
    isDefault: input.isDefault ?? false,
  });
  if (!res.address) throw new Error("update address failed");
  return mapAddress(res.address);
}

export async function deleteAddress(id: string): Promise<void> {
  await gateway().address.deleteAddress({ id });
}

export async function setDefaultAddress(id: string): Promise<ViewAddress> {
  const res = await gateway().address.setDefaultAddress({ id });
  if (!res.address) throw new Error("set default address failed");
  return mapAddress(res.address);
}
