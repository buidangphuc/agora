import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  type AddressInput,
  createAddress,
  deleteAddress,
  listAddresses,
  setDefaultAddress,
  updateAddress,
} from "./addresses.js";
import { makeClients } from "./client.js";

vi.mock("./client.js", () => ({ makeClients: vi.fn() }));
vi.mock("./session.js", () => ({
  getToken: vi.fn(() => "test-token"),
  SESSION_COOKIE: "session",
}));

function stubAddress(rpcs: Record<string, ReturnType<typeof vi.fn>>) {
  const address = {
    listAddresses: vi.fn(),
    createAddress: vi.fn(),
    updateAddress: vi.fn(),
    deleteAddress: vi.fn(),
    setDefaultAddress: vi.fn(),
    ...rpcs,
  };
  vi.mocked(makeClients).mockReturnValue({ address } as never);
  return address;
}

const protoAddress = {
  id: "a1",
  userId: "u1",
  recipientName: "Nguyen Van A",
  phone: "0900000000",
  street: "1 Main",
  ward: "W",
  district: "D",
  city: "HCM",
  isDefault: true,
};

const input: AddressInput = {
  recipientName: "Nguyen Van A",
  phone: "0900000000",
  street: "1 Main",
  city: "HCM",
};

beforeEach(() => vi.clearAllMocks());

describe("addresses gateway wrapper", () => {
  it("listAddresses maps proto addresses to view models", async () => {
    const address = stubAddress({
      listAddresses: vi.fn().mockResolvedValue({ addresses: [protoAddress] }),
    });
    const res = await listAddresses();
    expect(address.listAddresses).toHaveBeenCalledWith({});
    expect(res).toEqual([
      {
        id: "a1",
        userId: "u1",
        recipientName: "Nguyen Van A",
        phone: "0900000000",
        street: "1 Main",
        ward: "W",
        district: "D",
        city: "HCM",
        isDefault: true,
      },
    ]);
  });

  it("listAddresses normalizes errors to an empty list", async () => {
    stubAddress({
      listAddresses: vi.fn().mockRejectedValue(new Error("network")),
    });
    await expect(listAddresses()).resolves.toEqual([]);
  });

  it("createAddress maps optional fields to empty-string defaults", async () => {
    const address = stubAddress({
      createAddress: vi.fn().mockResolvedValue({ address: protoAddress }),
    });
    await createAddress(input);
    expect(address.createAddress).toHaveBeenCalledWith({
      recipientName: "Nguyen Van A",
      phone: "0900000000",
      street: "1 Main",
      ward: "",
      district: "",
      city: "HCM",
      isDefault: false,
    });
  });

  it("createAddress throws when the gateway returns no address", async () => {
    stubAddress({ createAddress: vi.fn().mockResolvedValue({}) });
    await expect(createAddress(input)).rejects.toThrow("create address failed");
  });

  it("updateAddress forwards the id and maps the input", async () => {
    const address = stubAddress({
      updateAddress: vi.fn().mockResolvedValue({ address: protoAddress }),
    });
    await updateAddress("a1", { ...input, isDefault: true });
    expect(address.updateAddress).toHaveBeenCalledWith({
      id: "a1",
      recipientName: "Nguyen Van A",
      phone: "0900000000",
      street: "1 Main",
      ward: "",
      district: "",
      city: "HCM",
      isDefault: true,
    });
  });

  it("deleteAddress and setDefaultAddress call their RPCs", async () => {
    const address = stubAddress({
      deleteAddress: vi.fn().mockResolvedValue({}),
      setDefaultAddress: vi.fn().mockResolvedValue({ address: protoAddress }),
    });
    await deleteAddress("a1");
    const def = await setDefaultAddress("a1");
    expect(address.deleteAddress).toHaveBeenCalledWith({ id: "a1" });
    expect(address.setDefaultAddress).toHaveBeenCalledWith({ id: "a1" });
    expect(def.isDefault).toBe(true);
  });
});
