import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createAddress,
  deleteAddress,
  setDefaultAddress,
  updateAddress,
} from "@/lib/gateway/addresses";

import {
  type AddressState,
  createAddressAction,
  deleteAddressAction,
  setDefaultAddressAction,
  updateAddressAction,
} from "./actions";

vi.mock("@/lib/gateway/addresses", () => ({
  createAddress: vi.fn(),
  updateAddress: vi.fn(),
  deleteAddress: vi.fn(),
  setDefaultAddress: vi.fn(),
}));

const initial: AddressState = { ok: false, message: "" };

function form(fields: Record<string, string>): FormData {
  const fd = new FormData();
  for (const [k, v] of Object.entries(fields)) fd.set(k, v);
  return fd;
}

const validFields = {
  recipientName: "Nguyen Van A",
  phone: "0900000000",
  street: "1 Main",
  city: "HCM",
};

beforeEach(() => vi.clearAllMocks());

describe("createAddressAction", () => {
  it("rejects incomplete input without calling the gateway", async () => {
    const res = await createAddressAction(
      initial,
      form({ recipientName: "A" }),
    );
    expect(res.ok).toBe(false);
    expect(res.message).toContain("bắt buộc");
    expect(createAddress).not.toHaveBeenCalled();
  });

  it("creates the address, revalidates, and returns success", async () => {
    vi.mocked(createAddress).mockResolvedValue({} as never);
    const res = await createAddressAction(initial, form(validFields));
    expect(createAddress).toHaveBeenCalledWith(
      expect.objectContaining({
        recipientName: "Nguyen Van A",
        phone: "0900000000",
        street: "1 Main",
        city: "HCM",
      }),
    );
    expect(revalidatePath).toHaveBeenCalledWith("/account/addresses");
    expect(res).toEqual({ ok: true, message: "Đã thêm địa chỉ thành công." });
  });

  it("returns the error message when the gateway throws", async () => {
    vi.mocked(createAddress).mockRejectedValue(new Error("duplicate"));
    const res = await createAddressAction(initial, form(validFields));
    expect(res).toEqual({ ok: false, message: "duplicate" });
  });
});

describe("updateAddressAction", () => {
  it("forwards the id and input on success", async () => {
    vi.mocked(updateAddress).mockResolvedValue({} as never);
    const res = await updateAddressAction("a1", initial, form(validFields));
    expect(updateAddress).toHaveBeenCalledWith(
      "a1",
      expect.objectContaining({ city: "HCM" }),
    );
    expect(res.ok).toBe(true);
  });
});

describe("delete/setDefault actions", () => {
  it("deleteAddressAction calls through and revalidates", async () => {
    vi.mocked(deleteAddress).mockResolvedValue(undefined);
    await deleteAddressAction("a1");
    expect(deleteAddress).toHaveBeenCalledWith("a1");
    expect(revalidatePath).toHaveBeenCalledWith("/checkout");
  });

  it("setDefaultAddressAction calls through", async () => {
    vi.mocked(setDefaultAddress).mockResolvedValue({} as never);
    await setDefaultAddressAction("a1");
    expect(setDefaultAddress).toHaveBeenCalledWith("a1");
  });
});
