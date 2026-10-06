import { revalidatePath } from "next/cache";
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createAddress,
  deleteAddress,
  setDefaultAddress,
  updateAddress,
} from "@/lib/gateway/addresses";

import {
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

const saved = { id: "a1", ...validFields } as never;

beforeEach(() => vi.clearAllMocks());

describe("createAddressAction", () => {
  it("rejects incomplete input without calling the gateway", async () => {
    const res = await createAddressAction(form({ recipientName: "A" }));
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error).toContain("bắt buộc");
    expect(createAddress).not.toHaveBeenCalled();
  });

  it("creates the address, revalidates, and returns it as data", async () => {
    vi.mocked(createAddress).mockResolvedValue(saved);
    const res = await createAddressAction(form(validFields));
    expect(createAddress).toHaveBeenCalledWith(
      expect.objectContaining({
        recipientName: "Nguyen Van A",
        phone: "0900000000",
        street: "1 Main",
        city: "HCM",
      }),
    );
    expect(revalidatePath).toHaveBeenCalledWith("/account/addresses");
    expect(res).toEqual({ ok: true, data: saved });
  });

  it("resolves with { ok: false, error } when the gateway throws", async () => {
    vi.mocked(createAddress).mockRejectedValue(new Error("duplicate"));
    const res = await createAddressAction(form(validFields));
    expect(res).toEqual({ ok: false, error: "duplicate" });
    expect(revalidatePath).not.toHaveBeenCalled();
  });
});

describe("updateAddressAction", () => {
  it("forwards the id and input on success", async () => {
    vi.mocked(updateAddress).mockResolvedValue(saved);
    const res = await updateAddressAction("a1", form(validFields));
    expect(updateAddress).toHaveBeenCalledWith(
      "a1",
      expect.objectContaining({ city: "HCM" }),
    );
    expect(res.ok).toBe(true);
  });

  it("resolves with an error when the gateway throws", async () => {
    vi.mocked(updateAddress).mockRejectedValue(new Error("nope"));
    const res = await updateAddressAction("a1", form(validFields));
    expect(res).toEqual({ ok: false, error: "nope" });
  });
});

describe("delete/setDefault actions", () => {
  it("deleteAddressAction calls through and revalidates both routes", async () => {
    vi.mocked(deleteAddress).mockResolvedValue(undefined);
    const res = await deleteAddressAction("a1");
    expect(deleteAddress).toHaveBeenCalledWith("a1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/addresses");
    expect(revalidatePath).toHaveBeenCalledWith("/checkout");
    expect(res).toEqual({ ok: true });
  });

  it("deleteAddressAction resolves with an error instead of throwing", async () => {
    vi.mocked(deleteAddress).mockRejectedValue(new Error("in use"));
    await expect(deleteAddressAction("a1")).resolves.toEqual({
      ok: false,
      error: "in use",
    });
  });

  it("setDefaultAddressAction calls through and revalidates", async () => {
    vi.mocked(setDefaultAddress).mockResolvedValue({} as never);
    const res = await setDefaultAddressAction("a1");
    expect(setDefaultAddress).toHaveBeenCalledWith("a1");
    expect(revalidatePath).toHaveBeenCalledWith("/account/addresses");
    expect(res).toEqual({ ok: true });
  });

  it("setDefaultAddressAction resolves with an error instead of throwing", async () => {
    vi.mocked(setDefaultAddress).mockRejectedValue(new Error("gateway down"));
    await expect(setDefaultAddressAction("a1")).resolves.toEqual({
      ok: false,
      error: "gateway down",
    });
  });
});
