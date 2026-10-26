import { render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { ViewAddress } from "@/lib/gateway/addresses";
import { listAddresses } from "@/lib/gateway/addresses";
import { calculateShippingFee } from "@/lib/gateway/orders";

import { ShippingInfo } from "./ShippingInfo";

vi.mock("@/lib/gateway/addresses", () => ({ listAddresses: vi.fn() }));
vi.mock("@/lib/gateway/orders", () => ({ calculateShippingFee: vi.fn() }));

function addr(id: string, city: string, isDefault: boolean): ViewAddress {
  return {
    id,
    userId: "u1",
    recipientName: "A",
    phone: "0900000000",
    street: "1 Đường A",
    ward: "",
    district: "",
    city,
    isDefault,
  };
}

async function row(signedIn: boolean) {
  const { container } = render(await ShippingInfo({ signedIn }));
  return container;
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(listAddresses).mockResolvedValue([]);
  vi.mocked(calculateShippingFee).mockResolvedValue(null);
});

describe("ShippingInfo (PDP shipping row)", () => {
  it("shows the city fee for a default address", async () => {
    vi.mocked(listAddresses).mockResolvedValue([
      addr("a1", "Đà Nẵng", false),
      addr("a2", "Hà Nội", true),
    ]);
    vi.mocked(calculateShippingFee).mockResolvedValue({
      shippingFee: 20000,
      isFreeShipping: false,
      message: "",
      estimatedDeliveryAt: null,
    });
    const c = await row(true);
    expect(calculateShippingFee).toHaveBeenCalledWith("Hà Nội", 0);
    expect(c).toHaveTextContent("Vận chuyển");
    expect(c).toHaveTextContent("500.000");
    expect(c).toHaveTextContent("Hà Nội");
    expect(c).toHaveTextContent("20.000");
    expect(c).not.toHaveTextContent("35.000");
    expect(c).not.toHaveTextContent("Đà Nẵng");
  });

  it("anonymous gets the range without a server call", async () => {
    const c = await row(false);
    expect(c).toHaveTextContent("500.000");
    expect(c).toHaveTextContent("20.000");
    expect(c).toHaveTextContent("35.000");
    expect(listAddresses).not.toHaveBeenCalled();
    expect(calculateShippingFee).not.toHaveBeenCalled();
  });

  it("no default address shows the range", async () => {
    vi.mocked(listAddresses).mockResolvedValue([addr("a1", "Đà Nẵng", false)]);
    const c = await row(true);
    expect(c).toHaveTextContent("500.000");
    expect(c).toHaveTextContent("20.000");
    expect(c).toHaveTextContent("35.000");
    expect(calculateShippingFee).not.toHaveBeenCalled();
  });

  it("no saved address at all shows the range", async () => {
    const c = await row(true);
    expect(c).toHaveTextContent("35.000");
    expect(calculateShippingFee).not.toHaveBeenCalled();
  });

  it("server failure shows the range", async () => {
    vi.mocked(listAddresses).mockResolvedValue([addr("a2", "Hà Nội", true)]);
    vi.mocked(calculateShippingFee).mockResolvedValue(null);
    const c = await row(true);
    expect(calculateShippingFee).toHaveBeenCalledTimes(1);
    expect(c).toHaveTextContent("500.000");
    expect(c).toHaveTextContent("20.000");
    expect(c).toHaveTextContent("35.000");
    expect(c).not.toHaveTextContent("Hà Nội");
  });
  it("shows the server delivery date (Checkout coherence: the PDP shows the server delivery date)", async () => {
    vi.mocked(listAddresses).mockResolvedValue([addr("a2", "Hà Nội", true)]);
    vi.mocked(calculateShippingFee).mockResolvedValue({
      shippingFee: 20000,
      isFreeShipping: false,
      message: "",
      estimatedDeliveryAt: "2026-10-18T10:00:00+07:00",
    });
    const c = await row(true);
    expect(c).toHaveTextContent("Nhận hàng dự kiến 18/10");
  });

  it("formats the date in Ho Chi Minh time (Checkout coherence: the PDP shows the server delivery date)", async () => {
    vi.mocked(listAddresses).mockResolvedValue([addr("a2", "Hà Nội", true)]);
    vi.mocked(calculateShippingFee).mockResolvedValue({
      shippingFee: 20000,
      isFreeShipping: false,
      message: "",
      estimatedDeliveryAt: "2026-10-17T18:30:00Z",
    });
    const c = await row(true);
    expect(c).toHaveTextContent("Nhận hàng dự kiến 18/10");
  });

  it("no delivery date without an address (Checkout coherence: the PDP shows no date without an address)", async () => {
    const anon = await row(false);
    expect(anon).not.toHaveTextContent("Nhận hàng dự kiến");
    expect(calculateShippingFee).not.toHaveBeenCalled();

    vi.mocked(listAddresses).mockResolvedValue([addr("a1", "Đà Nẵng", false)]);
    const noDefault = await row(true);
    expect(noDefault).not.toHaveTextContent("Nhận hàng dự kiến");
    expect(calculateShippingFee).not.toHaveBeenCalled();
  });

  it("failed or unset delivery date shows nothing (Checkout coherence: a failed or unset delivery date is not invented)", async () => {
    vi.mocked(listAddresses).mockResolvedValue([addr("a2", "Hà Nội", true)]);
    vi.mocked(calculateShippingFee).mockResolvedValue(null);
    const failed = await row(true);
    expect(failed).not.toHaveTextContent("Nhận hàng dự kiến");

    vi.mocked(calculateShippingFee).mockResolvedValue({
      shippingFee: 20000,
      isFreeShipping: false,
      message: "",
      estimatedDeliveryAt: null,
    });
    const unset = await row(true);
    expect(unset).toHaveTextContent("20.000");
    expect(unset).not.toHaveTextContent("Nhận hàng dự kiến");
  });
});
