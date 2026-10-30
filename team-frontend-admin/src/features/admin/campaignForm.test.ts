import { describe, expect, it } from "vitest";

import {
  EMPTY_DRAFT,
  formatCampaignTime,
  parseCampaignDraft,
  parseLocalDateTime,
} from "./campaignForm";

const NOW = new Date("2026-11-01T00:00:00.000Z");

const valid = {
  listingId: " l-1 ",
  variantId: "",
  salePrice: "99000",
  stockCap: "50",
  startsAt: "2026-11-01T08:00",
  endsAt: "2026-11-01T12:00",
};

describe("parseLocalDateTime", () => {
  it("reads a datetime-local value as Vietnam time", () => {
    expect(parseLocalDateTime("2026-11-01T08:00")?.toISOString()).toBe(
      "2026-11-01T01:00:00.000Z",
    );
  });

  it.each(["", "2026-11-01", "2026-11-01 08:00", "2026-13-40T99:99"])(
    "rejects %j",
    (raw) => {
      expect(parseLocalDateTime(raw)).toBeNull();
    },
  );
});

describe("parseCampaignDraft", () => {
  it("returns typed input with trimmed ids", () => {
    const res = parseCampaignDraft(valid, NOW);
    expect(res).toEqual({
      ok: true,
      value: {
        listingId: "l-1",
        variantId: "",
        salePrice: 99000,
        stockCap: 50,
        startsAt: new Date("2026-11-01T01:00:00.000Z"),
        endsAt: new Date("2026-11-01T05:00:00.000Z"),
      },
    });
  });

  it("flags every missing field on an empty draft", () => {
    const res = parseCampaignDraft(EMPTY_DRAFT, NOW);
    expect(res.ok).toBe(false);
    if (res.ok) return;
    expect(Object.keys(res.errors).sort()).toEqual([
      "endsAt",
      "listingId",
      "salePrice",
      "startsAt",
      "stockCap",
    ]);
  });

  it.each(["0", "-5", "1.5", "abc", "1e3", "99999999999999999999"])(
    "rejects price and stock %j",
    (n) => {
      const res = parseCampaignDraft(
        { ...valid, salePrice: n, stockCap: n },
        NOW,
      );
      expect(res.ok).toBe(false);
      if (res.ok) return;
      expect(res.errors.salePrice).toBeTruthy();
      expect(res.errors.stockCap).toBeTruthy();
    },
  );

  it("requires the end after the start", () => {
    const res = parseCampaignDraft(
      { ...valid, endsAt: "2026-11-01T08:00" },
      NOW,
    );
    expect(res).toEqual({
      ok: false,
      errors: { endsAt: "Thời điểm kết thúc phải sau thời điểm bắt đầu." },
    });
  });

  it("requires the end in the future", () => {
    const res = parseCampaignDraft(
      { ...valid, startsAt: "2026-10-01T08:00", endsAt: "2026-10-02T08:00" },
      NOW,
    );
    expect(res).toEqual({
      ok: false,
      errors: { endsAt: "Thời điểm kết thúc phải ở tương lai." },
    });
  });

  it("rejects ids over 200 bytes", () => {
    const long = "đ".repeat(101);
    const res = parseCampaignDraft(
      { ...valid, listingId: long, variantId: long },
      NOW,
    );
    expect(res.ok).toBe(false);
    if (res.ok) return;
    expect(res.errors.listingId).toBe("Mã tin đăng quá dài.");
    expect(res.errors.variantId).toBe("Mã phân loại quá dài.");
  });
});

describe("formatCampaignTime", () => {
  it("formats in Vietnam time", () => {
    expect(formatCampaignTime("2026-11-01T01:00:00.000Z")).toContain("08:00");
  });

  it("labels an open bound", () => {
    expect(formatCampaignTime("")).toBe("Không giới hạn");
  });
});
